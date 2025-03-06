package bencode

import (
	"errors"
	"io"
	"reflect"
	"strings"
)

// 转化struct/slice为文本
func Marshal(w io.Writer, s interface{}) int {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	return marshalValue(w, v)
}

func marshalValue(w io.Writer, v reflect.Value) int {
	len := 0
	switch v.Kind() {
	case reflect.String:
		len += EncodeString(w, v.String())
	case reflect.Int:
		len += EncodeInt(w, int(v.Int()))
	case reflect.Slice:
		len += marshalList(w, v)
	case reflect.Struct:
		len += marshalDict(w, v)
	}

	return len
}

func marshalDict(w io.Writer, v reflect.Value) int {
	len := 2
	// 写入dict开头
	w.Write([]byte{'d'})
	for i := range v.NumField() {
		fv := v.Field(i)
		ft := v.Type().Field(i)
		key := ft.Tag.Get("bencode")
		if key == "" {
			key = strings.ToLower(ft.Name)
		}
		len += EncodeString(w, key)
		len += marshalValue(w, fv)
	}
	w.Write([]byte{'e'})

	return len
}

func marshalList(w io.Writer, v reflect.Value) int {
	len := 2
	// 写入list开头
	w.Write([]byte{'l'})
	for i := range v.Len() {
		elem := v.Index(i)
		len += marshalValue(w, elem)
	}
	w.Write([]byte{'e'})

	return len
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
		// 传入s可能为空slice，直接把底层指针指向新创建slice，保证长度一致，不需要reflect append
		l := reflect.MakeSlice(p.Elem().Type(), len(list), len(list))
		p.Elem().Set(l)
		err = unmarshalList(p, list)
		if err != nil {
			return err
		}
	case BDICT:
		dict, _ := o.Dict()
		err = unmarshalDict(p, dict)
		if err != nil {
			return err
		}
	default:
		return errors.New("src is unsupported type")
	}

	return nil
}

func unmarshalDict(p reflect.Value, dict map[string]*BObject) error {
	// 校验参数 p 必须是指向结构体的指针
	if p.Kind() != reflect.Ptr || p.Elem().Type().Kind() != reflect.Struct {
		return errors.New("dest must be pointer")
	}

	// 获取指针指向的结构体
	v := p.Elem()
	// 遍历结构体的所有字段
	for i, n := 0, v.NumField(); i < n; i++ {
		// 获取当前字段的反射值
		reflectValue := v.Field(i)
		// 如果字段不可设置（比如未导出的私有字段），则跳过
		if !reflectValue.CanSet() {
			continue
		}

		// 获取字段的类型信息
		reflectType := v.Type().Field(i)
		// 获取字段的 bencode 标签，如果没有则使用字段名的小写形式
		key := reflectType.Tag.Get("bencode")
		if key == "" {
			key = strings.ToLower(reflectType.Name)
		}

		// 从 dict 中查找对应的值，如果没有则跳过
		reflectBObject := dict[key]
		if reflectBObject == nil {
			continue
		}

		// 根据 bencode 对象的类型进行相应的处理
		switch reflectBObject.type_ {
		case BSTR:
			// 如果字段类型是字符串，则设置字符串值
			if reflectType.Type.Kind() != reflect.String {
				break
			}
			val, _ := reflectBObject.Str()
			reflectValue.SetString(val)

		case BINT:
			// 如果字段类型是整数，则设置整数值
			if reflectType.Type.Kind() != reflect.Int {
				break
			}
			val, _ := reflectBObject.Int()
			reflectValue.SetInt(int64(val))

		case BLIST:
			// 如果字段类型是切片，则递归处理列表
			if reflectType.Type.Kind() != reflect.Slice {
				break
			}
			list, _ := reflectBObject.List()
			// 创建新的切片并设置到字段
			lp := reflect.New(reflectType.Type)
			ls := reflect.MakeSlice(reflectType.Type, len(list), len(list))
			lp.Elem().Set(ls)
			err := unmarshalList(lp, list)
			if err != nil {
				break
			}
			reflectValue.Set(lp.Elem())

		case BDICT:
			// 如果字段类型是结构体，则递归处理字典
			if reflectType.Type.Kind() != reflect.Struct {
				break
			}
			// 创建新的结构体并递归解析
			dp := reflect.New(reflectType.Type)
			dict, _ := reflectBObject.Dict()
			err := unmarshalDict(dp, dict)
			if err != nil {
				break
			}
			reflectValue.Set(dp.Elem())
		}
	}

	return nil
}

func unmarshalList(p reflect.Value, list []*BObject) error {
	if p.Kind() != reflect.Ptr || p.Elem().Type().Kind() != reflect.Slice {
		return errors.New("[unmarshalList] dest must be a pointer to slice")
	}

	// 拿到底层元素，其实就是长度为len(list)的空slice
	v := p.Elem()
	// 根据第一个元素判断，这个list内部装的是什么类型，只会走到一个case
	switch list[0].type_ {
	case BSTR:
		for i, o := range list {
			val, err := o.Str()
			if err != nil {
				return err
			}
			v.Index(i).SetString(val)
		}
	case BINT:
		for i, o := range list {
			val, err := o.Int()
			if err != nil {
				return err
			}
			v.Index(i).SetInt(int64(val))
		}
	case BLIST:
		for i, o := range list {
			// 获取当前元素的子列表
			val, err := o.List()
			if err != nil {
				return err
			}
			// 检查目标切片的元素类型是否也是切片，因为这里处理的是嵌套列表
			if v.Type().Elem().Kind() != reflect.Slice {
				return errors.New("[unmarshalList] list element type not match")
			}
			// 创建一个新的指针，指向目标切片类型
			lp := reflect.New(v.Type().Elem())
			// 创建一个新的切片实例，长度和容量都是子列表的长度
			ls := reflect.MakeSlice(v.Type().Elem(), len(val), len(val))
			// 将新创建的切片设置到指针指向的位置
			lp.Elem().Set(ls)
			// 递归处理子列表，将其解析到新创建的切片中
			err = unmarshalList(lp, val)
			if err != nil {
				return err
			}
			// 将处理好的子切片设置到父切片的对应位置
			v.Index(i).Set(lp.Elem())
		}
	case BDICT:
		for i, o := range list {
			val, err := o.Dict()
			if err != nil {
				return err
			}

			if v.Type().Elem().Kind() != reflect.Struct {
				return errors.New("[unmarshalList] list element type not match")
			}
			// 创建一个新的指针，指向目标结构体类型
			dp := reflect.New(v.Type().Elem())
			err = unmarshalDict(dp, val)
			if err != nil {
				return err
			}
			// 将处理好的子结构体设置到父切片的对应位置
			v.Index(i).Set(dp.Elem())
		}
	}

	return nil
}
