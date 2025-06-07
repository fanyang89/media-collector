package bilibili

import "testing"

func TestGetInnerText(t *testing.T) {
	txt := getInnerText(`<em class=\"keyword\">大黑塔</em>：我打我自己？`)
	if txt != "大黑塔：我打我自己？" {
		t.Fail()
	}
}
