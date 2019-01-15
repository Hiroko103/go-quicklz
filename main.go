// Fast data compression library
// Copyright (C) 2006-2011 Lasse Mikkel Reinhold
// lar@quicklz.com
//
// QuickLZ can be used for free under the GPL 1, 2 or 3 license (where anything
// released into public must be open source) or under a commercial license if such
// has been acquired (see http://www.quicklz.com/order.html). The commercial license
// does not cover derived or ported versions created by third parties under GPL.

// 1.5.0 final

// Golang port made by https://github.com/Hiroko103

package quicklz

import (
    "encoding/binary"
    "errors"
    "strconv"
)

const _VERSION_MAJOR = 1
const _VERSION_MINOR = 5
const _VERSION_REVISION = 0

const _MINOFFSET = 2
const _UNCONDITIONAL_MATCHLEN = 6
const _UNCOMPRESSED_END = 4
const _CWORD_LEN = 4

const (
    COMPRESSION_LEVEL_1 = 1
    COMPRESSION_LEVEL_2 = 2
    COMPRESSION_LEVEL_3 = 3
)

const (
    STREAMING_BUFFER_0 = 0
    STREAMING_BUFFER_100000 = 100000
    STREAMING_BUFFER_1000000 = 1000000
)

type Qlz struct {
    offset_base int64
    _POINTERS uint
    _HASH_VALUES uint
    _STREAMING_BUFFER uint
    _COMPRESSION_LEVEL uint
    state *state_compress
    state2 *state_decompress
}

type hash_compress struct {
    cache uint32    // QLZ_COMPRESSION_LEVEL == 1
    offset int64    // QLZ_COMPRESSION_LEVEL == 1 && QLZ_PTR_64 && QLZ_STREAMING_BUFFER == 0
    offset2 []int64 // QLZ_COMPRESSION_LEVEL != 1
}

type hash_decompress struct {
    offset int64
    offset2 []int64
}

type state_compress struct {
    stream_buffer []byte
    stream_counter int64
    hash []hash_compress
    hash_counter []byte
}

type state_decompress struct {
    stream_buffer []byte
    hash []hash_decompress
    hash_counter []byte
    stream_counter int64
}

// Create new compressor/decompressor
func New(compression_level uint, streaming_buffer uint) (*Qlz, error) {
    q := Qlz{}
    switch compression_level {
    case COMPRESSION_LEVEL_1:
        q._COMPRESSION_LEVEL = compression_level
        q._POINTERS = 1
        q._HASH_VALUES = 4096
    case COMPRESSION_LEVEL_2:
        q._COMPRESSION_LEVEL = compression_level
        q._POINTERS = 4
        q._HASH_VALUES = 2048
    case COMPRESSION_LEVEL_3:
        q._COMPRESSION_LEVEL = compression_level
        q._POINTERS = 16
        q._HASH_VALUES = 4096
    default:
        return &q, errors.New("invalid compression level (" + strconv.Itoa(int(compression_level)) + ")")
    }

    if streaming_buffer == STREAMING_BUFFER_0 ||
        streaming_buffer == STREAMING_BUFFER_100000 ||
        streaming_buffer == STREAMING_BUFFER_1000000 {
            q._STREAMING_BUFFER = streaming_buffer
    } else {
        return &q, errors.New("invalid streaming buffer size (" + strconv.Itoa(int(streaming_buffer)) + ")")
    }
    q.state = q.new_compress_state()
    q.state2 = q.new_decompress_state()

    if compression_level == COMPRESSION_LEVEL_1 && streaming_buffer == STREAMING_BUFFER_0 {
        q.offset_base = 0
    } else {
        q.offset_base = -1
    }

    q.reset_table_compress()
    q.reset_table_decompress()

    return &q, nil
}

func (q *Qlz) new_compress_state() *state_compress {
    state := state_compress{}
    if q._STREAMING_BUFFER > 0 {
        state.stream_buffer = make([]byte, q._STREAMING_BUFFER)
    }
    state.stream_counter = 0
    state.hash = make([]hash_compress, q._HASH_VALUES)
    if q._COMPRESSION_LEVEL != 1 {
        for i := uint(0); i < q._HASH_VALUES; i++ {
            state.hash[i].offset2 = make([]int64, q._POINTERS)
        }
    }
    state.hash_counter = make([]byte, q._HASH_VALUES)

    return &state
}

func (q *Qlz) new_decompress_state() *state_decompress {
    state := state_decompress{}
    if q._COMPRESSION_LEVEL == 1 || q._COMPRESSION_LEVEL == 2 {
        if q._STREAMING_BUFFER > 0 {
            state.stream_buffer = make([]byte, q._STREAMING_BUFFER)
        }
        state.hash = make([]hash_decompress, q._HASH_VALUES)
        for i := uint(0); i < q._HASH_VALUES; i++ {
            state.hash[i].offset2 = make([]int64, q._POINTERS)
        }
        state.hash_counter = make([]byte, q._HASH_VALUES)
    } else if q._COMPRESSION_LEVEL == 3 {
        if q._STREAMING_BUFFER > 0 {
            state.stream_buffer = make([]byte, q._STREAMING_BUFFER)
        }
        if q._COMPRESSION_LEVEL <= 2 {
            state.hash = make([]hash_decompress, q._HASH_VALUES)
            for i := uint(0); i < q._HASH_VALUES; i++ {
                state.hash[i].offset2 = make([]int64, q._POINTERS)
            }
        }

    }
    state.stream_counter = 0

    return &state
}

func (q *Qlz) reset_table_compress() {
    for i := uint(0); i < q._HASH_VALUES; i++ {
        if q._COMPRESSION_LEVEL == 1 {
            q.state.hash[i].offset = q.offset_base
        } else {
            q.state.hash_counter[i] = 0
        }
    }
}

func (q *Qlz) reset_table_decompress() {
    if q._COMPRESSION_LEVEL == 2 {
        for i := uint(0); i < q._HASH_VALUES; i++ {
            q.state2.hash_counter[i] = 0
        }
    }
}

// Compress the data in source to destination and return the compressed data length
func (q *Qlz) Compress(source, destination *[]byte) (int64, error) {
    if len(*source) == 0 || len(*destination) == 0 {
        return 0, errors.New("zero length buffer")
    }
    if len(*destination) < len(*source) + 400 {
        return 0, errors.New("destination buffer is smaller than the size of the source buffer + 400")
    }
    var r int64
    var compressed uint32
    var base int64
    size := int64(len(*source))

    if size == 0 || size > 0xffffffff - 400 {
        return 0, nil
    }

    if size < 216 {
        base = 3
    } else {
        base = 9
    }

    if q._STREAMING_BUFFER <= 0 || (q._STREAMING_BUFFER > 0 && q.state.stream_counter + size - 1 >= int64(q._STREAMING_BUFFER)) {
        q.reset_table_compress()
        r = base + q.compress_core(source, 0, destination, base, size)
        if q._STREAMING_BUFFER > 0 {
            q.reset_table_compress()
        }
        if r == base {
            copy((*destination)[base:base+size], *source)
            r = size + base
            compressed = 0
        } else {
            compressed = 1
        }
        q.state.stream_counter = 0
    } else if q._STREAMING_BUFFER > 0 && !(q.state.stream_counter + size - 1 >= int64(q._STREAMING_BUFFER)) {
        src_index := q.state.stream_counter
        copy(q.state.stream_buffer[src_index:], (*source)[:size])
        r = base + q.compress_core(&q.state.stream_buffer, src_index, destination, base, size)

        if r == base {
            copy((*destination)[base:], q.state.stream_buffer[src_index:src_index + size])
            r = size + base
            compressed = 0
            q.reset_table_compress()
        } else {
            compressed = 1
        }
        q.state.stream_counter += size
    }
    if base == 3 {
        (*destination)[0] = byte(0 | compressed)
        (*destination)[1] = byte(r)
        (*destination)[2] = byte(size)
    } else {
        (*destination)[0] = byte(2 | compressed)
        fast_write(uint32(r), destination, 1, 4)
        fast_write(uint32(size), destination, 5, 4)
    }

    (*destination)[0] |= byte(q._COMPRESSION_LEVEL << 2)
    (*destination)[0] |= 1 << 6
    if q._STREAMING_BUFFER == 0 {
        (*destination)[0] |= 0 << 4
    } else if q._STREAMING_BUFFER == 100000 {
        (*destination)[0] |= 1 << 4
    } else if q._STREAMING_BUFFER == 1000000 {
        (*destination)[0] |= 2 << 4
    } else {
        (*destination)[0] |= 3 << 4
    }

    return r, nil
}

// Decompress the data in source to destination and return the decompressed data length
func (q *Qlz) Decompress(source, destination *[]byte) (int64, error) {
    if len(*source) == 0 || len(*destination) == 0 {
        return 0, errors.New("zero length buffer")
    }
    dsiz := Size_decompressed(source)
    if int64(len(*destination)) < dsiz {
        return 0, errors.New("destination buffer size is smaller than source buffer")
    }

    if q._STREAMING_BUFFER <= 0 ||
       (q._STREAMING_BUFFER > 0 && q.state2.stream_counter + Size_decompressed(source) - 1 >= int64(q._STREAMING_BUFFER)) {
        if ((*source)[0] & 1) == 1 {
            q.reset_table_decompress()
            dsiz = q.decompress_core(source, 0, destination, 0, dsiz, 0)
        } else {
            header_size := size_header(source)
            copy(*destination, (*source)[header_size:header_size+dsiz])
        }
        q.state2.stream_counter = 0
        q.reset_table_decompress()
    } else if q._STREAMING_BUFFER > 0 {
        dst_index := q.state2.stream_counter
        if ((*source)[0] & 1) == 1 {
            dsiz = q.decompress_core(source, 0, &q.state2.stream_buffer, dst_index, dsiz, 0)
        } else {
            header_size := size_header(source)
            copy(q.state2.stream_buffer[dst_index:], (*source)[header_size:header_size+dsiz])
            q.reset_table_decompress()
        }
        copy(*destination, q.state2.stream_buffer[dst_index:dst_index+dsiz])
        q.state2.stream_counter += dsiz
    }
    return dsiz, nil
}

func (q *Qlz) compress_core(source *[]byte, src_index int64, destination *[]byte, dst_index int64, size int64) int64 {
    last_byte := size - 1
    src := int64(0)
    cword_ptr := int64(0)
    dst := int64(_CWORD_LEN)
    cword_val := uint32(1) << 31
    last_matchstart := last_byte - _UNCONDITIONAL_MATCHLEN - _UNCOMPRESSED_END
    fetch := uint32(0)
    lits := uint64(0)

    if src <= last_matchstart {
        fetch = fast_read(source, src+src_index, 3)
    }

    for src <= last_matchstart {
        if (cword_val & 1) == 1 {
            if src > (size >> 1) && dst > src - (src >> 5) {
                return 0
            }

            fast_write(cword_val >> 1 | (uint32(1) << 31), destination, cword_ptr + dst_index, _CWORD_LEN)

            cword_ptr = dst
            dst += _CWORD_LEN
            cword_val = uint32(1) << 31
        }
        if q._COMPRESSION_LEVEL == 1 {
            var o int64 = 0
            var hash uint32 = 0
            var cached uint32 = 0

            hash = q.hash_func(fetch)
            cached = fetch ^ q.state.hash[hash].cache
            q.state.hash[hash].cache = fetch

            o = q.state.hash[hash].offset
            q.state.hash[hash].offset = src + src_index

            if (cached & 0xffffff) == 0 && o != q.offset_base &&
               (src+src_index - o > _MINOFFSET || (src+src_index == o + 1 && lits >= 3 && src > 3 && same(source, src+src_index - 3, 6) == 1)) {
                if cached != 0 {
                    hash <<= 4
                    cword_val = (cword_val >> 1) | (uint32(1) << 31)
                    fast_write((3 - 2) | hash, destination, dst + dst_index, 2)
                    src += 3
                    dst += 2
                } else {
                    old_src := src
                    var matchlen int64 = 0
                    hash <<= 4

                    cword_val = (cword_val >> 1) | (uint32(1) << 31)
                    src += 4

                    if (*source)[o + src - old_src] == (*source)[src+src_index] {
                        src++
                        if (*source)[o + src - old_src] == (*source)[src+src_index] {
                            _q := last_byte - _UNCOMPRESSED_END - (src - 5) + 1
                            var remaining int64 = 0
                            if _q > 255 {
                                remaining = 255
                            } else {
                                remaining = _q
                            }
                            src++
                            for (*source)[o + src - old_src] == (*source)[src+src_index] && (src - old_src) < remaining {
                                src++
                            }
                        }
                    }

                    matchlen = src - old_src
                    if matchlen < 18 {
                        fast_write(uint32(matchlen - 2) | hash, destination, dst + dst_index, 2)
                        dst += 2
                    } else {
                        fast_write(uint32(matchlen << 16) | hash, destination, dst + dst_index, 3)
                        dst += 3
                    }
                }
                fetch = fast_read(source, src+src_index, 3)
                lits = 0
            } else {
                lits++
                (*destination)[dst+dst_index] = (*source)[src+src_index]
                src++
                dst++
                cword_val = cword_val >> 1
                fetch = fast_read(source, src+src_index, 3)
            }
        } else if q._COMPRESSION_LEVEL >= 2 {
            var o int64 = 0
            var offset2 int64 = 0
            var matchlen, m int64
            var hash, k, best_k uint32
            var c byte
            var remaining int64 = 0
            if last_byte-_UNCOMPRESSED_END-src+1 > 255 {
                remaining = 255
            } else {
                remaining = last_byte - _UNCOMPRESSED_END - src + 1
            }

            fetch = fast_read(source, src+src_index, 3)
            hash = q.hash_func(fetch)

            c = q.state.hash_counter[hash]

            offset2 = q.state.hash[hash].offset2[0]
            if offset2 + _MINOFFSET < src+src_index && c > 0 && ((fast_read(source, offset2, 3)^fetch)&0xffffff) == 0 {
                matchlen = 3
                if (*source)[offset2+matchlen] == (*source)[src+src_index+matchlen] {
                    matchlen = 4
                    for (*source)[offset2+matchlen] == (*source)[src+src_index+matchlen] && matchlen < remaining {
                        matchlen++
                    }
                }
            } else {
                matchlen = 0
            }
            for k = 1; k < uint32(q._POINTERS) && uint32(c) > k; k++ {
                o = q.state.hash[hash].offset2[k]
                if (q._COMPRESSION_LEVEL == 3 &&
                    (((fast_read(source, o, 3)^fetch)&0xffffff) == 0 && o < src+src_index-_MINOFFSET)) ||
                    (q._COMPRESSION_LEVEL == 2 &&
                        ((*source)[src+src_index+matchlen] == (*source)[o+matchlen] && ((fast_read(source, o, 3)^fetch)&0xffffff) == 0 && o < src+src_index-_MINOFFSET)) {
                    m = 3
                    for (*source)[o+m] == (*source)[src+src_index+m] && m < remaining {
                        m++
                    }
                    if (q._COMPRESSION_LEVEL == 3 && (
                        (m > matchlen) || (m == matchlen && o > offset2))) ||
                        (q._COMPRESSION_LEVEL == 2 &&
                            m > matchlen) {
                        offset2 = o
                        matchlen = m
                        best_k = k
                    }
                }
            }
            o = offset2
            q.state.hash[hash].offset2[c & byte(q._POINTERS-1)] = src+src_index
            c++
            q.state.hash_counter[hash] = c

            if q._COMPRESSION_LEVEL == 3 {
                if matchlen > 2 && src+src_index-o < 131071 {
                    u := int64(0)
                    offset := src+src_index - o

                    for u = 1; u < matchlen; u++ {
                        hash = q.hashat(source, src+src_index+u)
                        c = q.state.hash_counter[hash]
                        q.state.hash_counter[hash]++
                        q.state.hash[hash].offset2[c & byte(q._POINTERS-1)] = src + src_index + u
                    }

                    cword_val = (cword_val >> 1) | (uint32(1) << 31)
                    src += matchlen

                    if matchlen == 3 && offset <= 63 {
                        (*destination)[dst+dst_index] = byte(offset << 2)
                        dst++
                    } else if matchlen == 3 && offset <= 16383 {
                        f := (uint32(offset) << 2) | 1
                        fast_write(f, destination, dst+dst_index, 2)
                        dst += 2
                    } else if matchlen <= 18 && offset <= 1023 {
                        f := ((uint32(matchlen) - 3) << 2) | (uint32(offset) << 6) | 2
                        fast_write(f, destination, dst+dst_index, 2)
                        dst += 2
                    } else if matchlen <= 33 {
                        f := ((uint32(matchlen) - 2) << 2) | ((uint32(offset) << 7) | 3)
                        fast_write(f, destination, dst+dst_index, 3)
                        dst += 3
                    } else {
                        f := ((uint32(matchlen) - 3) << 7) | ((uint32(offset) << 15) | 3)
                        fast_write(f, destination, dst+dst_index, 4)
                        dst += 4
                    }
                } else {
                    (*destination)[dst+dst_index] = (*source)[src+src_index]
                    src++
                    dst++
                    cword_val = cword_val >> 1
                }
            } else if q._COMPRESSION_LEVEL == 2 {
                if matchlen > 2 {
                    cword_val = (cword_val >> 1) | (uint32(1) << 31)
                    src += matchlen

                    if matchlen < 10 {
                        f := best_k | ((uint32(matchlen) - 2) << 2) | (hash << 5)
                        fast_write(f, destination, dst+dst_index, 2)
                        dst += 2
                    } else {
                        f := best_k | (uint32(matchlen) << 16) | (hash << 5)
                        fast_write(f, destination, dst+dst_index, 3)
                        dst += 3
                    }
                } else {
                    (*destination)[dst+dst_index] = (*source)[src+src_index]
                    src++
                    dst++
                    cword_val = cword_val >> 1
                }
            }
        }
    }
    for src <= last_byte {
        if (cword_val & 1) == 1 {
            fast_write((cword_val >> 1) | (uint32(1) << 31), destination, cword_ptr + dst_index, _CWORD_LEN)
            cword_ptr = dst
            dst += _CWORD_LEN
            cword_val = uint32(1) << 31
        }
        if q._COMPRESSION_LEVEL < 3 && src <= last_byte - 3 {
            if q._COMPRESSION_LEVEL == 1 {
                var hash, fetch uint32
                fetch = fast_read(source, src+src_index, 3)
                hash = q.hash_func(fetch)
                q.state.hash[hash].offset = src + src_index
                q.state.hash[hash].cache = fetch
            } else if q._COMPRESSION_LEVEL == 2 {
                var hash uint32
                var c byte
                hash = q.hashat(source, src+src_index)
                c = q.state.hash_counter[hash]
                q.state.hash[hash].offset2[c & byte(q._POINTERS - 1)] = src + src_index
                c++
                q.state.hash_counter[hash] = c
            }
        }
        (*destination)[dst+dst_index] = (*source)[src+src_index]
        src++
        dst++
        cword_val = cword_val >> 1
    }

    for (cword_val & 1) != 1 {
        cword_val = cword_val >> 1
    }

    fast_write((cword_val >> 1) | (uint32(1) << 31), destination, cword_ptr + dst_index, _CWORD_LEN)

    if dst < 9 {
        return 9
    } else {
        return dst
    }
}

func (q *Qlz) decompress_core(source *[]byte, src_index int64, destination *[]byte, dst_index int64, size int64, history int64) int64 {
    src := size_header(source)
    dst := dst_index
    last_destination_byte := dst + size - 1
    cword_val := uint32(1)
    last_matchstart := last_destination_byte - _UNCONDITIONAL_MATCHLEN - _UNCOMPRESSED_END
    last_hashed := dst_index - 1
    last_source_byte := Size_compressed(source) - 1
    bitlut := []uint32{4, 0, 1, 0, 2, 0, 1, 0, 3, 0, 1, 0, 2, 0, 1, 0}

    for {
        var fetch uint32

        if cword_val == 1 {
            if src + _CWORD_LEN - 1 > last_source_byte {
                return 0
            }
            cword_val = fast_read(source, src, _CWORD_LEN)
            src += _CWORD_LEN
        }

        if src + 4 - 1 > last_source_byte {
            return 0
        }

        fetch = fast_read(source, src, 4)

        if (cword_val & 1) == 1 {
            var matchlen uint32
            offset2 := int64(0)

            if q._COMPRESSION_LEVEL == 1 {
                var hash uint32
                cword_val = cword_val >> 1
                hash = (fetch >> 4) & 0xfff
                offset2 = q.state2.hash[hash].offset

                if (fetch & 0xf) != 0 {
                    matchlen = (fetch & 0xf) + 2
                    src += 2
                } else {
                    matchlen = uint32((*source)[src+2])
                    src += 3
                }
            } else if q._COMPRESSION_LEVEL == 2 {
                var hash uint32
                var c byte
                cword_val = cword_val >> 1
                hash = (fetch >> 5) & 0x7ff
                c = byte(fetch & 0x3)
                offset2 = q.state2.hash[hash].offset2[c]

                if (fetch & 28) != 0 {
                    matchlen = ((fetch >> 2) & 0x7) + 2
                    src += 2
                } else {
                    matchlen = uint32((*source)[src+2])
                    src += 3
                }
            } else if q._COMPRESSION_LEVEL == 3 {
                var offset uint32
                cword_val = cword_val >> 1
                if (fetch & 3) == 0 {
                    offset = (fetch & 0xff) >> 2
                    matchlen = 3
                    src++
                } else if (fetch & 2) == 0 {
                    offset = (fetch & 0xffff) >> 2
                    matchlen = 3
                    src += 2
                } else if (fetch & 1) == 0 {
                    offset = (fetch & 0xffff) >> 6
                    matchlen = ((fetch >> 2) & 15) + 3
                    src += 2
                } else if (fetch & 127) != 3 {
                    offset = (fetch >> 7) & 0x1ffff
                    matchlen = ((fetch >> 2) & 0x1f) + 2
                    src += 3
                } else {
                    offset = fetch >> 15
                    matchlen = ((fetch >> 7) & 255) + 3
                    src += 4
                }

                offset2 = dst - int64(offset)

            }

            if offset2 < history || offset2 > dst - _MINOFFSET - 1 {
                return 0
            }

            if int64(matchlen) > last_destination_byte - dst - _UNCOMPRESSED_END + 1 {
                return 0
            }

            memcpy_up(destination, dst, destination, offset2, int64(matchlen))
            dst += int64(matchlen)

            if q._COMPRESSION_LEVEL <= 2 {
                q.update_hash_upto(destination, &last_hashed, dst - int64(matchlen))
                last_hashed = dst - 1
            }
        } else {
            if dst < last_matchstart {
                n := int64(bitlut[cword_val & 0xf])
                // Original C line
                // *(ui32 *)dst = *(ui32 *)src;
                // To replicate this, we can write:
                memcpy_up(destination, dst, source, src, 4 - 1)
                cword_val = cword_val >> uint32(n)
                dst += n
                src += n
                if q._COMPRESSION_LEVEL <= 2 {
                    q.update_hash_upto(destination, &last_hashed, dst - 3)
                }
            } else {
                for dst <= last_destination_byte {
                    if cword_val == 1 {
                        src += _CWORD_LEN
                        cword_val = 1 << 31
                    }
                    if src >= last_source_byte + 1 {
                        return 0
                    }

                    (*destination)[dst] = (*source)[src]
                    dst++
                    src++
                    cword_val = cword_val >> 1
                }
                if q._COMPRESSION_LEVEL <= 2 {
                    q.update_hash_upto(destination, &last_hashed, last_destination_byte - 3)
                }
                return size
            }
        }
    }
}

func fast_read(source *[]byte, index int64, bytes uint32) uint32 {
    if bytes >= 1 && bytes <= 4 {
        return binary.LittleEndian.Uint32((*source)[index:index+4])
    } else {
        return 0
    }
}

func fast_write(f uint32, dst *[]byte, dst_index int64, bytes uint64) {
    switch bytes {
    case 4:
        (*dst)[dst_index] = byte(0xff & f)
        (*dst)[dst_index + 1] = byte(0xff & (f >> 8))
        (*dst)[dst_index + 2] = byte(0xff & (f >> 16))
        (*dst)[dst_index + 3] = byte(f >> 24)
    case 3:
        (*dst)[dst_index] = byte(0xff & f)
        (*dst)[dst_index + 1] = byte(0xff & (f >> 8))
        (*dst)[dst_index + 2] = byte(0xff & (f >> 16))
        (*dst)[dst_index + 3] = byte(f >> 24)
    case 2:
        (*dst)[dst_index] = byte(0xff & f)
        (*dst)[dst_index + 1] = byte(0xff & (f >> 8))
    case 1:
        (*dst)[dst_index] = byte(f)
    }
}

// Get the size of the data in source buffer after decompression
func Size_decompressed(source *[]byte) int64 {
    var n uint32
    var r int64
    if ((*source)[0] & 2) == 2 {
        n = 4
    } else {
        n = 1
    }
    r = int64(fast_read(source, int64(1 + n), n))
    r = r & (0xffffffff >> ((4 - n)*8))
    return r
}

// Get the size of the compressed data in source buffer
func Size_compressed(source *[]byte) int64 {
    var n uint32
    var r int64
    if ((*source)[0] & 2) == 2 {
        n = 4
    } else {
        n = 1
    }
    r = int64(fast_read(source, 1, n))
    r = r & (0xffffffff >> ((4 - n)*8))
    return r
}

func size_header(source *[]byte) int64 {
    if ((*source)[0] & 2) == 2 {
        return 2 * 4 + 1
    } else {
        return 2 * 1 + 1
    }
}

func memcpy_up(destination *[]byte, dst_index int64, source *[]byte, src_index int64, n int64) {
    var f int64 = 0
    for {
        (*destination)[dst_index + f] = (*source)[src_index + f]
        (*destination)[dst_index + f + 1] = (*source)[src_index + f + 1]
        (*destination)[dst_index + f + 2] = (*source)[src_index + f + 2]
        (*destination)[dst_index + f + 3] = (*source)[src_index + f + 3]
        f += _MINOFFSET + 1
        if !(f < n) {
            break
        }
    }
}

func (q *Qlz) update_hash(source *[]byte, s int64) {
    if q._COMPRESSION_LEVEL == 1 {
        var hash uint32
        hash = q.hashat(source, s)
        q.state2.hash[hash].offset = s
        q.state2.hash_counter[hash] = 1
    } else if q._COMPRESSION_LEVEL == 2 {
        var hash uint32
        var c byte
        hash = q.hashat(source, s)
        c = q.state2.hash_counter[hash]
        q.state2.hash[hash].offset2[c & byte(q._POINTERS - 1)] = s
        c++
        q.state2.hash_counter[hash] = c
    }
}

func (q *Qlz) update_hash_upto(source *[]byte, lh *int64, max int64) {
    for *lh < max {
        *lh++
        q.update_hash(source, *lh)
    }
}

func (q *Qlz) hash_func(i uint32) uint32 {
    if q._COMPRESSION_LEVEL == 2 {
        return ((i >> 9) ^ (i >> 13) ^ i) & uint32(q._HASH_VALUES - 1)
    } else {
        return ((i >> 12) ^ i) & uint32(q._HASH_VALUES - 1)
    }
}

func same(src *[]byte, src_index int64, n int64) int {
    for n > 0 && (*src)[src_index + n] == (*src)[src_index] {
        n--
    }
    if n == 0 {
        return 1
    }
    return 0
}

func (q *Qlz) hashat(source *[]byte, src int64) uint32 {
    var fetch, hash uint32
    fetch = fast_read(source, src, 3)
    hash = q.hash_func(fetch)
    return hash
}
