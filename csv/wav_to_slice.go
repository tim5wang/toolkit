package csv

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/mitchellh/mapstructure"

	"github.com/json-iterator/go/extra"
)

func ReadRawsFromFile(filepath string) (raws []string) {
	file, err := os.OpenFile(filepath, os.O_RDWR, 0666)
	if err != nil {
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		panic(err)
	}
	var size = stat.Size()
	log.Printf("file size=%v", size)

	buf := bufio.NewReader(file)
	for {
		line, err := buf.ReadString('\n')
		line = strings.TrimSpace(line)
		if len(line) > 0 {
			raws = append(raws, line)
		}
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return
			}
		}
	}
	return
}

func GetBodyWithRetry(url string, n int) (io.ReadCloser, error) {
	var err error
	var resp *http.Response
	for n > 0 {
		resp, err = http.Get(url)
		if err == nil {
			return resp.Body, nil
		}
		n--
	}
	err = fmt.Errorf("download url error")
	return nil, err
}

// ParseCSVToStruct 把文件转化为 []struct
func ParseCSVToStruct(path string, slice interface{}, split string) error {
	raws := ReadRawsFromFile(path)
	return ParseRawsToMapStructSlice(raws, slice, split)
}

// ParseRawsToMapStructSlice 把文件行转化为 []struct
func ParseRawsToMapStructSlice(raws []string, slice interface{}, split string) error {
	t := reflect.TypeOf(slice)
	s := reflect.ValueOf(slice)
	sliceKind := t.Kind()
	if sliceKind != reflect.Ptr {
		return errors.New("slice should be a slice address")
	}
	t = t.Elem()
	s = s.Elem()
	if t.Kind() != reflect.Slice {
		return errors.New("slice should be slice")
	}
	eleKind := t.Elem().Kind()
	eleType := t.Elem()
	if eleKind == reflect.Ptr {
		eleType = eleType.Elem()
	}
	datas := ParseRawsToMapSlice(raws, split)
	for i, data := range datas {
		_, _ = i, data
		eleValue := reflect.New(eleType)
		// err := JsonFuzzyDecode(data, eleValue.Interface())
		err := mapStructByTagName(data, eleValue.Interface(), "json")
		if err != nil {
			return err
		}
		if eleKind != reflect.Ptr {
			eleValue = eleValue.Elem()
		}
		ns := reflect.Append(s, eleValue)
		s.Set(ns)
	}
	return nil
}

// ParseRawsToMapSlice 把文件行转化为 []map[string]string
func ParseRawsToMapSlice(raws []string, split string) []map[string]string {
	var indexFiled = "INDEX"
	var heads = []string{indexFiled}
	dataParsed := make([]map[string]string, 0, len(raws)-1)
	for ln, line := range raws {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		dataLine := strings.Split(line, split)
		if ln == 0 {
			for _, h := range dataLine {
				h = strings.TrimSpace(h)
				h = strings.Replace(h, "\uFEFF", "", 1)
				heads = append(heads, h)
			}
			continue
		}
		row := make(map[string]string)
		for i, v := range dataLine {
			if i >= len(heads)-1 {
				continue
			}
			row[indexFiled] = fmt.Sprintf("%v", ln)
			row[heads[i+1]] = v
		}
		dataParsed = append(dataParsed, row)
	}
	return dataParsed
}

func MapStructureDecode(src interface{}, target interface{}) error {
	return mapStructByTagName(src, target, "mapstructure")
}

// JsonFuzzyDecode json反序列化
func JsonFuzzyDecode(src interface{}, target interface{}) error {
	extra.RegisterFuzzyDecoders()
	return jsoniter.Unmarshal(ToBytes(src), target)
}

func mapStructByTagName(src interface{}, target interface{}, tagName string) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          tagName,
		Result:           target,
		WeaklyTypedInput: true,
	})
	if err != nil {
		return err
	}
	return decoder.Decode(src)
}

func SetFieldNameByTag(item interface{}, tagName string, fieldName string, value interface{}) error {
	v := reflect.ValueOf(item).Elem()
	if !v.CanAddr() {
		return fmt.Errorf("cannot assign to the item passed, item must be a pointer in order to assign")
	}
	findJsonName := func(t reflect.StructTag) (string, error) {
		if jt, ok := t.Lookup(tagName); ok {
			return strings.Split(jt, ",")[0], nil
		}
		return "", fmt.Errorf("tag provided does not define a json tag %v", fieldName)
	}
	fieldNames := map[string]int{}
	for i := 0; i < v.NumField(); i++ {
		typeField := v.Type().Field(i)
		tag := typeField.Tag
		jname, _ := findJsonName(tag)
		fieldNames[jname] = i
	}

	fieldNum, ok := fieldNames[fieldName]
	if !ok {
		return fmt.Errorf("field %s does not exist within the provided item", fieldName)
	}
	fieldVal := v.Field(fieldNum)
	fieldVal.Set(reflect.ValueOf(value))
	return nil
}

func ToBytes(obj interface{}) []byte {
	switch val := obj.(type) {
	case string:
		return []byte(val)
	case []byte:
		return val
	}
	bytes, _ := json.Marshal(obj)
	return bytes
}
