package MemoryPool

import (
	"golearning/Actor/MemoryPool/Common"
	"golearning/Actor/MemoryPool/GenericAllocator"
	"sync"
	"unsafe"
)

type BucketAllocator struct {
	pGenericInst Common.Allocator

	bucketsCount      uint32
	bucketSizeInBytes uint32
	bucketsDataBegin  [Common.MaxBucketCount]uintptr
	buckets           [Common.MaxBucketCount]PoolBucket
	pBuffer           uintptr
	pBufferEnd        uintptr
}

func Create(bucketsCount uint32, bucketSizeInBytes uint32) Common.Allocator {

	pGenericInstance := GenericAllocator.Create()

	if pGenericInstance == nil {
		return nil
	}

	align := Align(uint32(unsafe.Alignof(BucketAllocator{})), Common.Cache_Line_Size)
	bucketAlloc := (*BucketAllocator)(pGenericInstance.Alloc(uint32(unsafe.Sizeof(BucketAllocator{})), align))
	bucketAlloc.pGenericInst = pGenericInstance
	bucketAlloc.init(bucketsCount, bucketSizeInBytes)
	return bucketAlloc
}

func IsReadable(p unsafe.Pointer) bool {
	return uintptr(p) > Common.MaxValidAlignment
}

func Dump(alloc Common.Allocator) {

	if b, ok := alloc.(*BucketAllocator); ok {

		for i := 0; i < len(b.buckets); i++ {
			b.buckets[i].PrintAllRecord()
		}

	}
}

func Dispose() {

}

func Align(val uint32, alignment uint32) uint32 {
	r := (val + (alignment - 1)) & ^(alignment - 1)
	return r
}

func GetBucketIndexBySize(bytesCount uint32, strategy uint32) uint32 {
	var bucketIndex uint32
	switch strategy {
	default:
		panic("no proper strategy")
	case Common.Partioning_Strategy_Linear:
		bucketIndex = (bytesCount - 1) >> 4
	case Common.Partioning_Strategy_Piecewise_Linear:
		size := bytesCount - 1
		p0 := size >> 4
		p1 := 7 + (size >> 7)
		p2 := 13 + (size >> 9)

		if size <= 127 {
			bucketIndex = p0
		} else {
			if size > 1023 {
				bucketIndex = p2
			} else {
				bucketIndex = p1
			}
		}
	}

	return bucketIndex
}
func GetBucketSizeInBytesByIndex(bucketIndex uint32, strategy uint32) uint32 {
	var sizeInBytes uint32

	switch strategy {
	default:
		panic("no proper strategy")
	case Common.Partioning_Strategy_Linear:
		sizeInBytes = 16 + bucketIndex*16
	case Common.Partioning_Strategy_Piecewise_Linear:
		p0 := (bucketIndex + 1) << 4
		p1 := (bucketIndex - 6) << 7
		p2 := (bucketIndex - 12) << 9

		if bucketIndex <= 7 {
			sizeInBytes = p0
		} else {
			if bucketIndex > 14 {
				sizeInBytes = p2
			} else {
				sizeInBytes = p1
			}
		}
	}

	return sizeInBytes
}

var gobalRef [Common.MaxBucketCount]*sync.Map

func (b *BucketAllocator) init(_bucketsCount uint32, _bucketSizeInBytes uint32) {

	if _bucketsCount > Common.MaxBucketCount {
		_bucketsCount = Common.MaxBucketCount
	}

	b.bucketsCount = _bucketsCount
	b.bucketSizeInBytes = Align(_bucketSizeInBytes, Common.MaxValidAlignment)

	for i := 0; i < len(b.bucketsDataBegin); i++ {
		b.bucketsDataBegin[i] = uintptr(0)
	}

	totalBytesCount := b.bucketSizeInBytes * b.bucketsCount

	b.pBuffer = uintptr(b.pGenericInst.Alloc(totalBytesCount, Common.MaxValidAlignment))
	b.pBufferEnd = b.pBuffer + uintptr(totalBytesCount+1)

	for i := uint32(0); i < b.bucketsCount; i++ {
		var pbucket = &b.buckets[i]
		pbucket.RecordMap = &sync.Map{}
		gobalRef[i] = pbucket.RecordMap
		pbucket.pData = b.pBuffer + uintptr(i*b.bucketSizeInBytes)
		pbucket.pBufferEnd = pbucket.pData + uintptr(b.bucketSizeInBytes)

		bucketSizeInBytes := GetBucketSizeInBytesByIndex(i, Common.Partioning_Strategy_Piecewise_Linear)

		pbucket.Create(bucketSizeInBytes)
		b.bucketsDataBegin[i] = pbucket.pData

	}

}

type Number interface {
	int64 | int32 | uint32 | uint64
}

func minT[T Number](a, b T) T {
	if a < b {
		return a
	}
	return b
}
func IsAligned(v uint32, align uint32) bool {
	lowBits := v & (align - 1)
	return (lowBits == 0)
}

var ddebugMtx sync.Mutex

func (b *BucketAllocator) Alloc(_bytesCount uint32, align uint32) unsafe.Pointer {

	if _bytesCount == 0 {
		return unsafe.Pointer(uintptr(align))
	}
	_bytesCount += uint32(unsafe.Sizeof(uint64(0)))

	bytesCount := Align(_bytesCount, align)
	bucketIndex := GetBucketIndexBySize(bytesCount, Common.Partioning_Strategy_Piecewise_Linear)

	maxBucketIndex := minT(b.bucketsCount, bucketIndex+4)

	for bucketIndex < maxBucketIndex {
		//ddebugMtx.Lock()
		pRes := b.buckets[bucketIndex].Alloc()
		//ddebugMtx.Unlock()
		if pRes != nil {
			//ddebugMtx.Lock()
			//RecordAlloc(pRes, 2)
			//ddebugMtx.Unlock()

			return pRes
		}

		for {
			bucketIndex++
			if IsAligned(GetBucketSizeInBytesByIndex(bucketIndex, Common.Partioning_Strategy_Piecewise_Linear), align) {
				break
			}
		}
	}

	pRes := b.pGenericInst.Alloc(_bytesCount, align)
	//ddebugMtx.Lock()
	//RecordAlloc(pRes, 2)
	//ddebugMtx.Unlock()

	return pRes
}

func (b *BucketAllocator) _FindBucket(p unsafe.Pointer) uint32 {
	index := uintptr(p) - b.bucketsDataBegin[0]
	return uint32(index) / b.bucketSizeInBytes
}

func (b *BucketAllocator) Free(p unsafe.Pointer) {
	//减去记录头
	p = unsafe.Pointer(uintptr(p) - unsafe.Sizeof(uint64(0)))
	bucketIndex := b._FindBucket(p)

	if bucketIndex < b.bucketsCount {

		pbucket := &b.buckets[bucketIndex]

		pbucket.FreeInterval(p, p)
		//ddebugMtx.Lock()
		//EraseRecord(p)
		//ddebugMtx.Unlock()
		return
	}

	p = unsafe.Pointer(uintptr(p) + unsafe.Sizeof(uint64(0)))
	b.pGenericInst.Free(p)

}

//go:noescape
//go:linkname memmove runtime.memmove
func memmove(to, from unsafe.Pointer, n uintptr)
func (b *BucketAllocator) Realloc(p unsafe.Pointer, size uint32, align uint32) unsafe.Pointer {

	if !IsReadable(p) {
		return b.Alloc(size, align)
	}

	if size == 0 {
		b.Free(p)
		return nil
	}

	bucketIndex := b._FindBucket(p)

	if bucketIndex < b.bucketsCount {
		elemSize := GetBucketIndexBySize(bucketIndex, Common.Partioning_Strategy_Piecewise_Linear)
		if size <= elemSize {
			return p
		}

		p2 := b.Alloc(size, align)
		if p2 == nil {
			return nil
		}

		if IsReadable(p) {
			memmove(p2, p, uintptr(elemSize))
		}

		b.Free(p)
		return p2
	}

	//if p was allocated from memory GenericAllocator
	if size == 0 {

		if IsReadable(p) {
			b.pGenericInst.Free(p)
		}

		return nil
	}

	return b.pGenericInst.Realloc(p, size, align)
}
