package bilibili

import (
	"testing"
)

func TestConvertAidToBvid(t *testing.T) {
	for _, test := range []struct {
		a int
		b string
	}{
		{a: 99999999, b: "BV1y7411Q7Eq"},
		{a: 170001, b: "BV17x411w7KC"},
		{a: 455017605, b: "BV1Q541167Qg"},
		{a: 882584971, b: "BV1mK4y1C7Bz"},
	} {
		a := test.a
		b := test.b
		if b != convertAidToBvid(a) {
			t.Fail()
		}
	}
}
