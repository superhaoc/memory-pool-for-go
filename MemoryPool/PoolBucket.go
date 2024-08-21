package MemoryPool

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

type TaggedIndex struct {
	tag    uint32
	offset uint32
}

const Invalid = ^uint64(0)

type u TaggedIndex

type PoolBucket struct {
	head       uint64  //atomic variable storage for tag and offset
	pData      uintptr //the allocated space must be non-GCed memory
	pBufferEnd uintptr
	globalTag  uint32 //atomic variable storage for recording tag for lock free

	RecordMap *sync.Map
}

func NewPoolBucket() *PoolBucket {
	return &PoolBucket{head: Invalid, pData: uintptr(0), pBufferEnd: uintptr(0), globalTag: 0}
}

func (b *PoolBucket) Create(elementSize uint32) {
	atomic.StoreUint32(&b.globalTag, 0)
	var node = uintptr(b.pData)
	var headVal TaggedIndex
	headVal.tag = atomic.LoadUint32(&b.globalTag)
	headVal.offset = (uint32)(node - uintptr(b.pData))
	atomic.StoreUint64(&b.head, *(*uint64)(unsafe.Pointer(&headVal)))

	for {
		var next = node + uintptr(elementSize)

		if next+uintptr(elementSize) <= uintptr(b.pBufferEnd) {
			var nextVal TaggedIndex

			nextVal.tag = atomic.AddUint32(&b.globalTag, 1) //atomic.LoadUint32(&b.globalTag)
			nextVal.offset = (uint32)(next - uintptr(b.pData))

			tagidx := (*TaggedIndex)(unsafe.Pointer(node))
			*tagidx = nextVal

		} else {
			*(*uint64)(unsafe.Pointer(node)) = Invalid
			break
		}

		node = next

	}
}
func IsLittleEndian() bool {
	byteSeq := [2]byte{0x1, 0x2}
	return *(*uint16)(unsafe.Pointer(&byteSeq[0])) == uint16(0x0201)
}

func (b *PoolBucket) RecordAlloc(p uintptr, skip int) {
	_, file, line, ok := runtime.Caller(skip)
	if ok {
		s := fmt.Sprintf("%s:%d\n", file, line)
		//if _, ok := RecordMap.Load(p); ok {
		//	return
		//}
		b.RecordMap.Store(p, s)
	}
}
func (b *PoolBucket) EraseRecord(p uintptr) {
	b.RecordMap.Delete(p)
}

func (b *PoolBucket) PrintAllRecord() {
	b.RecordMap.Range(func(key, value interface{}) bool {
		fmt.Print(value.(string))
		return true
	})
}

func (b *PoolBucket) Alloc() unsafe.Pointer {
	var p unsafe.Pointer = nil
	var headValue *TaggedIndex

	for {
		// assuming current platform is little-endian
		headValUint64 := atomic.LoadUint64(&b.head)
		headValue = (*TaggedIndex)(unsafe.Pointer(&headValUint64))

		if headValUint64 == Invalid {
			return nil
		}
		//allocate memory chunk
		p = unsafe.Pointer(b.pData + uintptr(headValue.offset))
		var nextValue = *(*uint64)(p)
		//nextHead := (*TaggedIndex)(p)
		//nextHead.tag = atomic.AddUint32(&b.globalTag, 1)

		if atomic.CompareAndSwapUint64(&b.head, headValUint64, nextValue) {
			b.RecordAlloc(uintptr(headValUint64), 3)
			//unlink info
			//*(*uint64)(p) = 0

			atomic.StoreUint64((*uint64)(p), headValUint64)
			p = unsafe.Pointer(uintptr(p) + unsafe.Sizeof(uint64(0)))

			break
		}
	}

	return p
}

func (b *PoolBucket) FreeInterval(_pHead, _pTail unsafe.Pointer) {
	pHead, pTail := uintptr(_pHead), uintptr(_pTail)

	//newTag := atomic.LoadUint32(&b.globalTag)
	newTag := atomic.AddUint32(&b.globalTag, 1)

	var nodeValue TaggedIndex
	//reconstruct link info
	nodeValue.offset = uint32(pHead - b.pData)
	nodeValue.tag = newTag

	pheadTag := *(*uint64)(_pHead)
	//fmt.Println(nodeValue.offset, "  ", pheadTag.offset)

	for {
		headValUint64 := atomic.LoadUint64(&b.head)
		headValue := (*TaggedIndex)(unsafe.Pointer(&headValUint64))
		if headValue.offset == nodeValue.offset {
			break
		}
		//put free memory chunk onto head, and link previous head
		*(*TaggedIndex)(unsafe.Pointer(pTail)) = *headValue
		if atomic.CompareAndSwapUint64(&b.head, headValUint64, *(*uint64)(unsafe.Pointer(&nodeValue))) {
			*(*uint32)(unsafe.Pointer(uintptr(_pHead) + 16 + 8)) = 0
			b.EraseRecord(uintptr(pheadTag))
			break
		}
	}

}
