// pycstrings 从 Python 2.7 marshal 格式 .pyc 中提取字符串常量，输出包含关键词的文件。
// 用法: pycstrings <dir> <keyword1> [keyword2...]
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Python 2.7 marshal 类型码
const (
	typeNull      = '0'
	typeNone      = 'N'
	typeFalse     = 'F'
	typeTrue      = 'T'
	typeStopIter  = 'S'
	typeEllipsis  = '.'
	typeInt       = 'i'
	typeInt64     = 'I'
	typeFloat     = 'f'
	typeComplex   = 'x'
	typeLong      = 'l'
	typeString    = 's'
	typeInterned  = 't'
	typeStringRef = 'R'
	typeTuple     = '('
	typeList      = '['
	typeDict      = '{'
	typeCode      = 'c'
	typeUnicode   = 'u'
	typeUnknown   = '?'
	typeSet       = '<'
	typeFrozenSet = '>'
)

func readString(data []byte, pos *int) string {
	n := int(binary.LittleEndian.Uint32(data[*pos:]))
	*pos += 4
	s := string(data[*pos : *pos+n])
	*pos += n
	return s
}

// walk 遍历 marshal 对象，收集所有字符串
func walk(data []byte, pos *int, depth int, out *[]string) {
	if depth > 20 || *pos >= len(data) {
		return
	}
	if *pos >= len(data) {
		return
	}
	t := data[*pos]
	*pos++
	switch t {
	case typeNone, typeFalse, typeTrue, typeStopIter, typeEllipsis, typeNull:
	case typeInt:
		*pos += 4
	case typeInt64:
		*pos += 8
	case typeFloat:
		*pos += 8
	case typeComplex:
		*pos += 16
	case typeLong:
		n := int(binary.LittleEndian.Uint32(data[*pos:]))
		*pos += 4 + n*2
	case typeString:
		s := readString(data, pos)
		*out = append(*out, s)
	case typeInterned:
		s := readString(data, pos)
		*out = append(*out, s)
	case typeStringRef:
		*pos += 4
	case typeUnicode:
		s := readString(data, pos)
		*out = append(*out, s)
	case typeTuple, typeList:
		n := int(binary.LittleEndian.Uint32(data[*pos:]))
		*pos += 4
		for i := 0; i < n; i++ {
			walk(data, pos, depth+1, out)
		}
	case typeDict:
		for {
			if *pos >= len(data) || data[*pos] == typeNull {
				*pos++
				break
			}
			walk(data, pos, depth+1, out)
			walk(data, pos, depth+1, out)
		}
	case typeSet, typeFrozenSet:
		n := int(binary.LittleEndian.Uint32(data[*pos:]))
		*pos += 4
		for i := 0; i < n; i++ {
			walk(data, pos, depth+1, out)
		}
	case typeCode:
		// code 对象: argcount, nlocals, stacksize, flags (4x int32)
		*pos += 16
		// code (string), consts (tuple), names (tuple), varnames (tuple),
		// freevars, cellvars, filename (string), name (string), firstlineno (int),
		// lnotab (string)
		walk(data, pos, depth+1, out) // code string
		walk(data, pos, depth+1, out) // consts tuple
		walk(data, pos, depth+1, out) // names tuple
		walk(data, pos, depth+1, out) // varnames tuple
		walk(data, pos, depth+1, out) // freevars
		walk(data, pos, depth+1, out) // cellvars
		walk(data, pos, depth+1, out) // filename
		walk(data, pos, depth+1, out) // name
		*pos += 4                     // firstlineno
		walk(data, pos, depth+1, out) // lnotab
	default:
		// 未知类型，无法安全跳过，放弃此分支
	}
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "用法: pycstrings <dir> <keyword1> [keyword2...]")
		os.Exit(2)
	}
	dir := os.Args[1]
	keywords := os.Args[2:]
	files, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".pyc") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil || len(data) < 16 {
			continue
		}
		// 跳过 pyc 头 12 字节 (magic 4 + flags 4 + timestamp 4) 或 16 字节
		start := 12
		if data[0] == 0x03 && data[1] == 0xf3 {
			// py2.7: magic(4) + flags(4)? 实际 03 f3 0d 0a 00 00 00 00 = magic+flags
			start = 12
		}
		pos := start
		var strs []string
		walk(data, &pos, 0, &strs)
		for _, kw := range keywords {
			for _, s := range strs {
				if strings.Contains(strings.ToLower(s), strings.ToLower(kw)) {
					fmt.Printf("%s: %q\n", f.Name(), s)
				}
			}
		}
	}
}
