// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package types

import (
	"errors"
	"flag"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/urfave/cli"
)

var (
	_ cli.Flag              = (*BigFlag)(nil)
	_ cli.RequiredFlag      = (*BigFlag)(nil)
	_ cli.DocGenerationFlag = (*BigFlag)(nil)
)

// BigFlag is a command line flag that accepts 256 bit big integers in decimal or
// hexadecimal syntax.
type BigFlag struct {
	Name string

	Category    string
	DefaultText string
	Usage       string
	EnvVar      string

	Required   bool
	Hidden     bool
	HasBeenSet bool

	Value *big.Int

	Aliases []string
}

// For cli.Flag:
func (f *BigFlag) GetName() string { return f.Name }
func (f *BigFlag) Names() []string { return append([]string{f.Name}, f.Aliases...) }
func (f *BigFlag) IsSet() bool     { return f.HasBeenSet }
func (f *BigFlag) String() string  { return cli.FlagStringer(f) }

func (f *BigFlag) Apply(set *flag.FlagSet) {
	eachName(f, func(name string) {
		f.Value = new(big.Int)
		set.Var((*bigValue)(f.Value), f.Name, f.Usage)
	})
}

// For cli.RequiredFlag:

func (f *BigFlag) IsRequired() bool { return f.Required }

// For cli.VisibleFlag:

func (f *BigFlag) IsVisible() bool { return !f.Hidden }

// For cli.CategorizableFlag:

func (f *BigFlag) GetCategory() string { return f.Category }

// For cli.DocGenerationFlag:

func (f *BigFlag) TakesValue() bool     { return true }
func (f *BigFlag) GetUsage() string     { return f.Usage }
func (f *BigFlag) GetValue() string     { return f.Value.String() }
func (f *BigFlag) GetEnvVars() []string { return nil } // env not supported

func (f *BigFlag) GetDefaultText() string {
	if f.DefaultText != "" {
		return f.DefaultText
	}
	return f.GetValue()
}

// bigValue turns *big.Int into a flag.Value
type bigValue big.Int

func (b *bigValue) String() string {
	if b == nil {
		return ""
	}
	return (*big.Int)(b).String()
}

func (b *bigValue) Set(s string) error {
	intVal, ok := math.ParseBig256(s)
	if !ok {
		return errors.New("invalid integer syntax")
	}
	*b = (bigValue)(*intVal)
	return nil
}

func eachName(f cli.Flag, fn func(string)) {
	parts := strings.SplitSeq(f.GetName(), ",")
	for name := range parts {
		name = strings.Trim(name, " ")
		fn(name)
	}
}

// GlobalBig returns the value of a BigFlag from the global flag set.
func GlobalBig(ctx *cli.Context, name string) *big.Int {
	val := ctx.Generic(name)
	if val == nil {
		return nil
	}
	return (*big.Int)(val.(*bigValue))
}
