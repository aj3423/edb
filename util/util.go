package util

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/sha3"
)

var ZeroAddress common.Address // all zeroes

// hex encoded in json, instead of base64
type ByteSlice []byte

func (s ByteSlice) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(s))
}
func (s *ByteSlice) UnmarshalJSON(data []byte) error {
	var str string
	e := json.Unmarshal(data, &str)
	if e != nil {
		return e
	}

	bs, e := hex.DecodeString(str)
	if e != nil {
		return e
	}
	*s = bs
	return nil
}

func MapToStruct(in, out interface{}) error {
	buf := new(bytes.Buffer)
	if e := json.NewEncoder(buf).Encode(in); e != nil {
		return e
	}
	return json.NewDecoder(buf).Decode(out)
}

type comparable interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 |
		float32 | float64
}

func Max[T comparable](x, y T) T {
	if x > y {
		return x
	}
	return y
}

func Min[T comparable](x, y T) T {
	if x < y {
		return x
	}
	return y
}

func ReverseSlice[T any](s []T) {
	sort.SliceStable(s, func(i, j int) bool {
		return i > j
	})
}
func CloneSlice[T any](s []T) []T {
	return append(s[:0:0], s...)
}

func HexEnc(data []byte) string {
	return fmt.Sprintf("%x", data)
}
func HexDec(data string) []byte {
	decoded, _ := hex.DecodeString(data)
	return decoded
}

func FileWrite(fn string, data []byte) error {
	return ioutil.WriteFile(fn, data, 0666)
}
func FileWriteStr(fn string, data string) error {
	return FileWrite(fn, []byte(data))
}
func FileExist(fn string) bool {
	_, err := os.Stat(fn)
	if err == nil || os.IsExist(err) {
		return true

	} else {
		return false
	}
}

func Sha3(bs []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(bs)
	return hash.Sum(nil)
}
