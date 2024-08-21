package Common

import (
	"sync/atomic"
	"unsafe"
)

// copy struct from runtime
type Type struct {
	Size_       uintptr
	PtrBytes    uintptr // number of (prefix) bytes in the type that can contain pointers
	Hash        uint32  // hash of type; avoids computation in hash tables
	TFlag       uint8   // extra type information flags
	Align_      uint8   // alignment of variable with this type
	FieldAlign_ uint8   // alignment of struct field with this type
	Kind_       uint8   // enumeration for C
	// function for comparing objects of this type
	// (ptr to object A, ptr to object B) -> ==?
	Equal func(unsafe.Pointer, unsafe.Pointer) bool
	// GCData stores the GC type data for the garbage collector.
	// If the KindGCProg bit is set in kind, GCData is a GC program.
	// Otherwise it is a ptrmask bitmap. See mbitmap.go for details.
	GCData    *byte
	Str       int32 // string form
	PtrToThis int32 // type for pointer to this type, may be zero
}

// copy struct from runtime
type Interface struct {
	Type *Type
	Data unsafe.Pointer
}

type Item struct {
	key   string
	value any
}

type SimpleHash struct {
	u     uint32
	m     uint64
	r     int32
	items *[]Item
}

const (
	_LoadFactor   = 0.5
	_InitCapacity = 1 // must be a power of 2
)

//go:noescape
//go:linkname strhash runtime.strhash
func strhash(_ unsafe.Pointer, _ uintptr) uintptr

func StrHash(s string) uint64 {
	if v := strhash(unsafe.Pointer(&s), 0); v == 0 {
		return 1
	} else {
		return uint64(v)
	}
}

func NewSimpleHash() *SimpleHash {
	ret := &SimpleHash{
		u:     0,
		m:     _InitCapacity,
		items: new([]Item),
		r:     int32(0),
	}

	*ret.items = make([]Item, _InitCapacity)
	return ret
}

func (s *SimpleHash) _insert(h uint64, key string, value interface{}) bool {
	p := h & (s.m - 1)
	for i := uint64(0); i <= s.m; i++ {

		oldu := atomic.LoadUint32(&s.u)
		oldm := atomic.LoadUint64(&s.m)

		if float64(oldu+1)/float64(oldm) > _LoadFactor {
			return false
		}

		pitem := (*[]Item)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.items))))

		if b := &(*pitem)[p]; b.value != nil {
			p += 1
			p &= s.m - 1 //共享
		} else {

			if atomic.CompareAndSwapUint32(&s.u, oldu, oldu+1) { //会出现丢失key/value的问题
				b.value = value
				b.key = key
				//atomic.AddUint32(&s.u, 1) //1000 c0,当前只有100
				return true
			}

		}
	}

	return false
}
func (s *SimpleHash) Put(key string, value interface{}) {
	h := StrHash(key)

	//grow
	for {
		//oldu := atomic.LoadUint32(&s.u)
		//oldm := atomic.LoadUint64(&s.m)
		//
		//if float64(oldu+1)/float64(oldm) > _LoadFactor {
		//
		//
		//
		//}

		if s._insert(h, key, value) {
			break
		} else {

			oldm := atomic.LoadUint64(&s.m)
			if atomic.CompareAndSwapInt32(&s.r, 0, 1) {

				pitem := new([]Item)
				*pitem = make([]Item, oldm*2)

				poldItem := (*[]Item)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.items)), unsafe.Pointer(pitem)))
				atomic.StoreUint32(&s.u, 0)
				atomic.StoreUint64(&s.m, oldm*2)

				for i := uint64(0); i < oldm; i++ {
					if b := (*poldItem)[i]; b.value != nil {

						s._insert(h, b.key, b.value)
						//
						//if !debug {
						//	break
						//}
					}
				}
				atomic.StoreInt32(&s.r, 0)
			} else {
				continue
			}
		}
	}

}

func (s *SimpleHash) Get(key interface{}) interface{} {
	return nil
}
