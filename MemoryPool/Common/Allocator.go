package Common

import "unsafe"

// all allocator interface based
type Allocator interface {
	Alloc(bytesCount uint32, align uint32) unsafe.Pointer
	Free(p unsafe.Pointer)
	Realloc(p unsafe.Pointer, size uint32, align uint32) unsafe.Pointer
}
