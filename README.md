# go-quicklz

[![GoDoc](https://godoc.org/github.com/Hiroko103/go-quicklz?status.svg)](https://godoc.org/github.com/Hiroko103/go-quicklz)

go-quicklz is a complete port of QuickLZ compression algorithm in go.

The official website for QuickLZ is available [here](http://www.quicklz.com/).

## Installation

```
go get github.com/Hiroko103/go-quicklz
```

## Using

```
import "github.com/Hiroko103/go-quicklz"
```

## Features

* Supports all **compression levels** and **buffering modes**
* Memory safe

## Notices

Be aware that compressing files with `STREAMING_BUFFER_0` mode will require an equal sized buffer.

If you plan to compress large files (>100 MB), use either `STREAMING_BUFFER_100000` or `STREAMING_BUFFER_1000000` mode.

## Examples

File compress

```go
qlz, err := quicklz.New(quicklz.COMPRESSION_LEVEL_1, quicklz.STREAMING_BUFFER_0)
if err != nil {
    fmt.Println(err)
    return
}
f, _ := os.Open(filePath)
source, _ := ioutil.ReadAll(f)
f.Close()
destination := make([]byte, len(source) + 400)
compressed_size, err := qlz.Compress(&source, &destination)
if err != nil {
    fmt.Println(err)
    return
}
compressed_data := destination[:compressed_size]
ioutil.WriteFile("outputFile", compressed_data, 0777)
```

File decompress

```go
qlz, err := quicklz.New(quicklz.COMPRESSION_LEVEL_1, quicklz.STREAMING_BUFFER_0)
if err != nil {
    fmt.Println(err)
    return
}
f, _ := os.Open(compressedFile)
source, _ := ioutil.ReadAll(f)
f.Close()
decompressed_size := quicklz.Size_decompressed(&source)
destination := make([]byte, decompressed_size)
decompressed_size, err = qlz.Decompress(&source, &destination)
if err != nil {
    fmt.Println(err)
    return
}
ioutil.WriteFile("originalFile", destination, 0777)
```

Stream compress

```go
qlz, err := quicklz.New(quicklz.COMPRESSION_LEVEL_1, quicklz.STREAMING_BUFFER_100000)
if err != nil {
    fmt.Println(err)
    return
}
f, _ := os.Open(filePath)
source := make([]byte, 10000)
destination := make([]byte, len(source) + 400)
of, _ := os.Create("compressedStream")
for {
    n, err := f.Read(source)
    if err != nil {
        if err == io.EOF {
            break
        }
    }
    part := source[:n]
    compressed_size, _ := qlz.Compress(&part, &destination)
    of.Write(destination[:compressed_size])
}
f.Close()
of.Close()
```

Stream decompress

```go
qlz, err := quicklz.New(quicklz.COMPRESSION_LEVEL_1, quicklz.STREAMING_BUFFER_100000)
if err != nil {
    fmt.Println(err)
    return
}
f, _ := os.Open(compressedStream)
source := make([]byte, 10000 + 400)
destination := make([]byte, 10000)
of, _ := os.Create("originalFile")
for {
    header := source[:9]
    n, err := f.Read(header)
    if err != nil {
        if err == io.EOF {
            break
        }
    }
    if n != 9 {
        break
    }
    c := quicklz.Size_compressed(&header)
    f.Read(source[9:c])
    part := source[:c]
    d, _ := qlz.Decompress(&part, &destination)
    of.Write(destination[:d])
}
f.Close()
of.Close()
```

## Credit

All credit goes to Lasse Mikkel Reinhold (lar@quicklz.com), the author of the original C version.

## License

This software is released under the GNU General Public License v3.0.

>  Permissions of this strong copyleft license are conditioned on making available complete source code of licensed works and modifications, which include larger works using a licensed work, under the same license. Copyright and license notices must be preserved. Contributors provide an express grant of patent rights.

Refer to the [LICENSE](https://github.com/Hiroko103/go-quicklz/blob/master/LICENSE) file for the complete text.
