package g79client

import (
	"strconv"
)

// 基础响应结构体
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
}

// 由于网易的节点代码差异，导致部分响应字段可能返回字符串/浮点数/数字/布尔/空字符串，所以需要一个通用类型来处理

// for int
// * 0
// * "0"
// * "" (0)
// * "false" (0)
// * "(other)" (0)

// for bool
// * true
// * "true"
// * 1 (true)
// * "1" (true)
// * "" (false)
// * "(other)" (false)

// 兼容服务端数值字段可能返回字符串/浮点数/数字/布尔/空字符串的类型
type Uncertain struct {
	Raw    string `json:"-"`
	Quoted bool   `json:"-"`
}

func (v *Uncertain) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		v.Raw = ""
		v.Quoted = false
		return nil
	}
	if b[0] == '"' {
		s, err := strconv.Unquote(string(b))
		if err != nil {
			return err
		}
		v.Raw = s
		v.Quoted = true
		return nil
	}
	// 数字或其他字面量
	v.Raw = string(b)
	v.Quoted = false
	if v.Raw == "null" {
		v.Raw = ""
		v.Quoted = true
	}
	return nil
}

func (v Uncertain) String() string {
	return v.Raw
}

func (v Uncertain) Float64() float64 {
	if v.Raw == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v.Raw, 64)
	if err != nil {
		return 0
	}
	return f
}

func (v Uncertain) Int64() int64 {
	if v.Raw == "" {
		return 0
	}
	i, err := strconv.ParseInt(v.Raw, 10, 64)
	if err != nil {
		return 0
	}
	return i
}

func (v Uncertain) Bool() bool {
	if v.Raw == "" {
		return false
	}
	b, err := strconv.ParseBool(v.Raw)
	if err != nil {
		return false
	}
	return b
}

func (v Uncertain) MarshalJSON() ([]byte, error) {
	if v.Quoted {
		return []byte(strconv.Quote(v.Raw)), nil
	}
	if v.Raw == "" {
		return []byte("\"\""), nil
	}
	if _, err := strconv.ParseFloat(v.Raw, 64); err == nil {
		return []byte(v.Raw), nil
	}
	return []byte(strconv.Quote(v.Raw)), nil
}
