package main

import (
	"bufio"
	"errors"
	"io"
	"reflect"
)

type BType uint8

const (
	BSTR  BType = 0x01
	BINT  BType = 0x02
	BLIST BType = 0x03
	BDICT BType = 0x04
)

type BValue interface{}

type BObject struct {
	type_ BType
	val_  BValue
}

// torrent -> struct 参数s就是传入的结构体指针，函数用来把torrent的数据填充到s中，所以要传入指针
// 先把文本解析为BObject，再把BObject填充到struct中
func Unmarshal(r io.Reader, s interface{}) error {
	// 解析文件文本为BObject
	o, err := Parse(r)
	if err != nil {
		return err
	}
	p := reflect.ValueOf(s)
	if p.Kind() != reflect.Ptr {
		return errors.New("dest must be a pointer")
	}

	switch o.type_ {
	case BLIST:
		list, _ := o.List()
		l := reflect.MakeSlice(p.Elem().Type(), len(list), len(list))
		p.Elem().Set(l)

	case BDICT:

	default:
		return errors.New("src is unsupported type")
	}

	return nil
}

// 返回val
func (o *BObject) Str() (string, error) {
	if o.type_ != BSTR {
		return "", errors.New("type is not string")
	}

	return o.val_.(string), nil
}

func (o *BObject) Int() (int, error) {
	if o.type_ != BINT {
		return 0, errors.New("type is not int")
	}

	return o.val_.(int), nil
}

func (o *BObject) List() ([]*BObject, error) {
	if o.type_ != BLIST {
		return nil, errors.New("type is not list")
	}

	return o.val_.([]*BObject), nil
}

func (o *BObject) Dict() (map[string]*BObject, error) {
	if o.type_ != BDICT {
		return nil, errors.New("type is not dict")
	}

	return o.val_.(map[string]*BObject), nil
}

// Bencode序列化
func (o *BObject) Bencode(w io.Writer) int {
	bw, ok := w.(*bufio.Writer)
	if !ok {
		bw = bufio.NewWriter(w)
	}

	len := 0
	switch o.type_ {
	case BSTR:
		str, _ := o.Str()
		len += EncodeString(bw, str)
	case BINT:
		num, _ := o.Int()
		len += EncodeInt(bw, num)
	case BLIST:
		bw.WriteByte('l')
		list, _ := o.List()
		for _, elem := range list {
			len += elem.Bencode(bw)
		}
		bw.WriteByte('e')
		len += 2
	case BDICT:
		bw.WriteByte('d')
		dict, _ := o.Dict()
		for k, v := range dict {
			len += EncodeString(bw, k)
			len += v.Bencode(bw)
		}
		bw.WriteByte('e')
		len += 2
	}
	bw.Flush()

	return len
}

// Bencode反序列化
func Parse(r io.Reader) (*BObject, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	first, err := br.Peek(1)
	if err != nil {
		return nil, err
	}
	var res BObject
	switch {
	case first[0] >= '0' && first[0] <= '9':
		// parse string
		val, err := DecodeString(br)
		if err != nil {
			return nil, err
		}
		res.type_ = BSTR
		res.val_ = val
	case first[0] == 'i':
		// parse int
		val, err := DecodeInt(br)
		if err != nil {
			return nil, err
		}
		res.type_ = BINT
		res.val_ = val
	case first[0] == 'l':
		// parse list
		br.ReadByte()
		var list []*BObject
		for {
			// 读到e，说明list解析结束
			if b, _ := br.Peek(1); b[0] == 'e' {
				br.ReadByte()
				break
			}
			elem, err := Parse(br)
			if err != nil {
				return nil, err
			}
			list = append(list, elem)
		}
		res.type_ = BLIST
		res.val_ = list
	case first[0] == 'd':
		// parse dict
		br.ReadByte()
		dict := make(map[string]*BObject)
		for {
			// 读到e，说明dict解析结束
			if b, _ := br.Peek(1); b[0] == 'e' {
				br.ReadByte()
				break
			}
			key, err := DecodeString(br)
			if err != nil {
				return nil, err
			}
			elem, err := Parse(br)
			if err != nil {
				return nil, err
			}
			dict[key] = elem
		}
		res.type_ = BDICT
		res.val_ = dict
	}

	return &res, nil
}

func EncodeString(w io.Writer, val string) int {
	strLen := len(val)
	bw := bufio.NewWriter(w)

	// 写入字符串长度
	len := writeDecimal(bw, strLen)
	// 写入分隔符
	bw.WriteByte(':')
	len++
	bw.WriteString(val)
	len += strLen

	err := bw.Flush()
	if err != nil {
		return 0
	}

	return len
}

func DecodeString(r io.Reader) (val string, err error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	num, len, _ := readDecimal(br)
	if len == 0 {
		return "", errors.New("invalid string")
	}
	b, _ := br.ReadByte()
	if b != ':' {
		return val, errors.New("invalid string")
	}
	buf := make([]byte, num)
	// 格式验证正确后，把后续string内容读出来
	_, err = io.ReadAtLeast(br, buf, num)
	val = string(buf)

	return
}

func EncodeInt(w io.Writer, val int) int {
	bw := bufio.NewWriter(w)
	len := 0

	bw.WriteByte('i')
	len++
	len += writeDecimal(bw, val)
	bw.WriteByte('e')
	len++

	err := bw.Flush()
	if err != nil {
		return 0
	}

	return len
}

func DecodeInt(r io.Reader) (val int, err error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	// 开头i
	first, _ := br.ReadByte()
	if first != 'i' {
		return 0, errors.New("invalid int")
	}
	val, _, _ = readDecimal(br)
	// 结尾e
	last, _ := br.ReadByte()
	if last != 'e' {
		return 0, errors.New("invalid int")
	}

	return
}

func writeDecimal(w *bufio.Writer, val int) (len int) {
	if val == 0 {
		w.WriteByte('0')
		len++
		return
	}

	// 处理负数
	if val < 0 {
		w.WriteByte('-')
		len++
		val = -val
	}

	// 根据val的位数，确定除数
	dividend := 1
	for {
		if dividend > val {
			dividend /= 10
			break
		}
		dividend *= 10
	}
	for {
		num := byte(val / dividend)
		// asic码方式写入
		w.WriteByte('0' + num)
		len++
		if dividend == 1 {
			break
		}
		val = val % dividend
		dividend /= 10
	}

	return
}

func readDecimal(r *bufio.Reader) (val int, len int, err error) {
	first, err := r.ReadByte()
	if err != nil {
		return 0, 0, err
	}
	// 处理负数
	nagative := false
	if first == '-' {
		nagative = true
		first, err = r.ReadByte()
		if err != nil {
			return 0, 0, err
		}
	}

	// 检查第一个数字是否合法
	if first < '0' || first > '9' {
		return 0, 0, errors.New("invalid first byte")
	}

	len = 0
	val = int(first - '0')
	for {
		b, err := r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, err
		}
		if b < '0' || b > '9' {
			r.UnreadByte()
			break
		}
		len++
		val = val*10 + int(b-'0')
	}
	if nagative {
		val = -val
	}

	return val, len, nil
}
