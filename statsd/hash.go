package main

func hash(name string) uint32 {
	var x uint64
	for _, ch := range name {
		for i := 15; i >= 0; i-- {
			x <<= 1
			x ^= 0x1edc6f41 * ((x >> 32) ^ (uint64(ch)>>uint(i))&1)
		}
	}
	return uint32(x & 0xffff)
}
