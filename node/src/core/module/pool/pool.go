package pool

import "sync"

var ByteSlicePool = sync.Pool{
    New: func() any {
        b := make([]byte, 0, 2048)
        return &b
    },
}

func GetBuffer(size int) *[]byte {
    p := ByteSlicePool.Get().(*[]byte)
    if cap(*p) < size {
        p2 := make([]byte, size)
        p = &p2
    }
    *p = (*p)[:0]
    return p
}

func PutBuffer(p *[]byte) {
    if cap(*p) > 10*1024*1024 {
        return
    }
    ByteSlicePool.Put(p)
}
