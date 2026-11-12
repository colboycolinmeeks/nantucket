// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package debugger

import (
	"testing"

	"nantucket/internal/vm"
)

func TestParseBP(t *testing.T) {
	for _, c := range []struct {
		arg, target, cond string
		hit               vm.HitCount
		rule              bool
		bad               bool
	}{
		{arg: "12", target: "12"},
		{arg: "", target: ""},
		{arg: "Calc", target: "Calc"},
		{arg: "12 IF x > 1", target: "12", cond: "x > 1", rule: true},
		{arg: "IF x > 1", target: "", cond: "x > 1", rule: true},
		{arg: "12 HIT 3", target: "12", hit: vm.HitCount{Op: '=', N: 3}, rule: true},
		{arg: "12 hit >= 3", target: "12", hit: vm.HitCount{Op: '>', N: 3}, rule: true},
		{arg: "HIT % 4", target: "", hit: vm.HitCount{Op: '%', N: 4}, rule: true},
		{arg: "Calc IF n < 0 HIT 2", target: "Calc", cond: "n < 0", hit: vm.HitCount{Op: '=', N: 2}, rule: true},
		{arg: "12 HIT 2 IF n < 0", target: "12", cond: "n < 0", hit: vm.HitCount{Op: '=', N: 2}, rule: true},
		// a variable called hit in the condition
		{arg: "12 IF hit > 3", target: "12", cond: "hit > 3", rule: true},
		{arg: "12 IF hit > 3 HIT 2", target: "12", cond: "hit > 3", hit: vm.HitCount{Op: '=', N: 2}, rule: true},
		{arg: "12 HIT", bad: true},
		{arg: "12 HIT 0", bad: true},
		{arg: "12 HIT x", bad: true},
		{arg: "12 IF", bad: true},
	} {
		target, cond, hit, rule, err := parseBP(c.arg)
		if c.bad {
			if err == nil {
				t.Errorf("%q: no error", c.arg)
			}
			continue
		}
		if err != nil || target != c.target || cond != c.cond || hit != c.hit || rule != c.rule {
			t.Errorf("%q: got %q %q %+v %v %v", c.arg, target, cond, hit, rule, err)
		}
	}
}

func TestParseLP(t *testing.T) {
	for _, c := range []struct {
		arg, target, msg, cond string
		hit                    vm.HitCount
		bad                    bool
	}{
		{arg: `12 "i={i}"`, target: "12", msg: "i={i}"},
		{arg: `"i={i}"`, msg: "i={i}"},
		{arg: `'say "hi"'`, msg: `say "hi"`},
		{arg: `Calc "x"`, target: "Calc", msg: "x"},
		{arg: `Calc() total {n}`, target: "Calc()", msg: "total {n}"},
		{arg: `total is {n}`, msg: "total is {n}"},
		{arg: `12 check if {x} is set`, target: "12", msg: "check if {x} is set"},
		{arg: `12`, target: "12"},
		{arg: `Calc`, target: "Calc"},
		{arg: ``},
		{arg: `12 "x" IF i > 1 HIT 2`, target: "12", msg: "x", cond: "i > 1", hit: vm.HitCount{Op: '=', N: 2}},
		{arg: `"x" HIT % 3`, msg: "x", hit: vm.HitCount{Op: '%', N: 3}},
		{arg: `12 "x" junk`, bad: true},
		{arg: `12 "x`, bad: true},
		{arg: `12 "x" HIT 0`, bad: true},
	} {
		target, msg, cond, hit, err := parseLP(c.arg)
		if c.bad {
			if err == nil {
				t.Errorf("%q: no error", c.arg)
			}
			continue
		}
		if err != nil || target != c.target || msg != c.msg || cond != c.cond || hit != c.hit {
			t.Errorf("%q: got %q %q %q %+v %v", c.arg, target, msg, cond, hit, err)
		}
	}
}
