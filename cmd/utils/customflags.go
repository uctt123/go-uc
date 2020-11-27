















package utils

import (
	"encoding"
	"errors"
	"flag"
	"math/big"
	"os"
	"os/user"
	"path"
	"strings"

	"github.com/ethereum/go-ethereum/common/math"
	"gopkg.in/urfave/cli.v1"
)




type DirectoryString string

func (s *DirectoryString) String() string {
	return string(*s)
}

func (s *DirectoryString) Set(value string) error {
	*s = DirectoryString(expandPath(value))
	return nil
}



type DirectoryFlag struct {
	Name   string
	Value  DirectoryString
	Usage  string
	EnvVar string
}

func (f DirectoryFlag) String() string {
	return cli.FlagStringer(f)
}



func (f DirectoryFlag) Apply(set *flag.FlagSet) {
	eachName(f.Name, func(name string) {
		set.Var(&f.Value, f.Name, f.Usage)
	})
}

func (f DirectoryFlag) GetName() string {
	return f.Name
}

func (f *DirectoryFlag) Set(value string) {
	f.Value.Set(value)
}

func eachName(longName string, fn func(string)) {
	parts := strings.Split(longName, ",")
	for _, name := range parts {
		name = strings.Trim(name, " ")
		fn(name)
	}
}

type TextMarshaler interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
}


type textMarshalerVal struct {
	v TextMarshaler
}

func (v textMarshalerVal) String() string {
	if v.v == nil {
		return ""
	}
	text, _ := v.v.MarshalText()
	return string(text)
}

func (v textMarshalerVal) Set(s string) error {
	return v.v.UnmarshalText([]byte(s))
}


type TextMarshalerFlag struct {
	Name   string
	Value  TextMarshaler
	Usage  string
	EnvVar string
}

func (f TextMarshalerFlag) GetName() string {
	return f.Name
}

func (f TextMarshalerFlag) String() string {
	return cli.FlagStringer(f)
}

func (f TextMarshalerFlag) Apply(set *flag.FlagSet) {
	eachName(f.Name, func(name string) {
		set.Var(textMarshalerVal{f.Value}, f.Name, f.Usage)
	})
}


func GlobalTextMarshaler(ctx *cli.Context, name string) TextMarshaler {
	val := ctx.GlobalGeneric(name)
	if val == nil {
		return nil
	}
	return val.(textMarshalerVal).v
}



type BigFlag struct {
	Name   string
	Value  *big.Int
	Usage  string
	EnvVar string
}


type bigValue big.Int

func (b *bigValue) String() string {
	if b == nil {
		return ""
	}
	return (*big.Int)(b).String()
}

func (b *bigValue) Set(s string) error {
	int, ok := math.ParseBig256(s)
	if !ok {
		return errors.New("invalid integer syntax")
	}
	*b = (bigValue)(*int)
	return nil
}

func (f BigFlag) GetName() string {
	return f.Name
}

func (f BigFlag) String() string {
	return cli.FlagStringer(f)
}

func (f BigFlag) Apply(set *flag.FlagSet) {
	eachName(f.Name, func(name string) {
		set.Var((*bigValue)(f.Value), f.Name, f.Usage)
	})
}


func GlobalBig(ctx *cli.Context, name string) *big.Int {
	val := ctx.GlobalGeneric(name)
	if val == nil {
		return nil
	}
	return (*big.Int)(val.(*bigValue))
}






func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		if home := HomeDir(); home != "" {
			p = home + p[1:]
		}
	}
	return path.Clean(os.ExpandEnv(p))
}

func HomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}
