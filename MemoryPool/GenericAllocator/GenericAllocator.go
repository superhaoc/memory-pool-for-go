package GenericAllocator

import (
	"golearning/Actor/MemoryPool/Common"
	"sync"
	"unsafe"
)

// golang native memory allocator,which used for supporting this memory pool
// manually manage memory, not GCed
//
//go:linkname sysAlloc runtime.sysAlloc
func sysAlloc(n uintptr, sysStat *sysMemStat) unsafe.Pointer

//go:linkname sysFree runtime.sysFree
func sysFree(v unsafe.Pointer, n uintptr, sysStat *sysMemStat)

//go:noescape
//go:linkname memmove runtime.memmove
func memmove(to, from unsafe.Pointer, n uintptr)

type sysMemStat uint64

type Header struct {
	p      uintptr //original allocated memory base address
	size   uint32
	offset uint32
}

type GenericAllocator struct {
	sysStat sysMemStat
}

var (
	allocator *GenericAllocator
	once      sync.Once
)

func Create() Common.Allocator {
	once.Do(func() {
		var s sysMemStat
		//dont use new syntax
		allocator = (*GenericAllocator)(sysAlloc(unsafe.Sizeof(GenericAllocator{}), &s))
	})
	return allocator
}

func Dispose() {
	if allocator != nil {
		var s sysMemStat
		sysFree(unsafe.Pointer(allocator), unsafe.Sizeof(GenericAllocator{}), &s)
	}
}

func (g *GenericAllocator) Alloc(bytesCount uint32, align uint32) unsafe.Pointer {
	if align < Common.MinValidAlignment {
		align = Common.MinValidAlignment
	}

	offset := align - 1 + uint32(unsafe.Sizeof(Header{}))

	//p := make([]byte, bytesCount+offset)
	p := sysAlloc(uintptr(bytesCount+offset), &g.sysStat)
	if p == nil {
		panic("can not allocate header")
	}

	//align assignment to the expected boundary
	p2 := (uintptr(p) + uintptr(offset)) & ^(uintptr(align) - 1)
	h := (*Header)(unsafe.Pointer(p2 - unsafe.Sizeof(Header{})))

	h.p = uintptr(p)
	h.offset = offset
	h.size = bytesCount
	return unsafe.Pointer(p2)
}

func (g *GenericAllocator) Free(p unsafe.Pointer) {

	if p == nil {
		return
	}

	h := (*Header)(unsafe.Pointer(uintptr(p) - unsafe.Sizeof(Header{})))
	sysFree(unsafe.Pointer(h.p), uintptr(h.size+h.offset), &g.sysStat)
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

func (g *GenericAllocator) Realloc(p unsafe.Pointer, size uint32, align uint32) unsafe.Pointer {
	p2 := g.Alloc(size, align)
	if p2 == nil {
		return nil
	}

	if p != nil {
		h := (*Header)(unsafe.Pointer(uintptr(p) - unsafe.Sizeof(Header{})))
		odlBlockSize := h.size + h.offset
		memmove(p2, p, uintptr(minT(size, odlBlockSize)))
	}
	return nil
}

func (g *GenericAllocator) GetUsableSpace(p unsafe.Pointer) uint32 {
	if p == nil {
		return 0
	}

	h := (*Header)(unsafe.Pointer(uintptr(p) - unsafe.Sizeof(Header{})))
	return h.size
}
