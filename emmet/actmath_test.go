package emmet

import "testing"

func TestActEvalMath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"10*3+2|", "32|"},
		{"width: 10*3|", "width: 30|"},
		{"‹(1+2)*3›", "9|"},
		{"a ‹ 2*4 › b", "a  8|  b"},
		{"7\\2|", "3|"},
		{"10/3|", "3.3333|"},
		{"2.5*2|", "5|"},
		{"0.1+0.2|", "0.3|"},
		{"1/0|", ""},
		{"item-10+5|", "item-15|"},
		{"-3*2|", "-6|"},
		{"x = (2+3)*(4-1)|;", "x = 15|;"},
		{"7%4|", "3|"},
		{"10*3 |", "30| "},
		{"42|", ""},
		{"abc|", ""},
		{"2*|", ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		ch, ok := EvalMath(text, sel)
		if got := actResult(t, text, ch, ok); got != c.want {
			t.Errorf("EvalMath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestActIncrementNumber(t *testing.T) {
	cases := []struct {
		in    string
		delta float64
		want  string
	}{
		{"margin: 10|px", 1, "margin: ‹11›px"},
		{"margin: |10px", 10, "margin: ‹20›px"},
		{"1.5|", 0.1, "‹1.6›"},
		{"1.50|", 1, "‹2.50›"},
		{"007|", 1, "‹008›"},
		{"099|", 1, "‹100›"},
		{"-1|", 1, "‹0›"},
		{"0|", -1, "‹-1›"},
		{"foo-5|", 1, "foo-‹6›"},
		{"x: -5|", 10, "x: ‹5›"},
		{"x: .5|", 0.1, "x: ‹.6›"},
		{"0.1|", 0.2, "‹0.3›"},
		{"10|", -0.1, "‹9.9›"},
		{"a ‹42› b", -2, "a ‹40› b"},
		{"abc|", 1, ""},
		{"v1.2.3|", 1, ""},
		{"end of sentence 10.|", 1, "end of sentence ‹11›."},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		ch, ok := IncrementNumber(text, sel, c.delta)
		if got := actResult(t, text, ch, ok); got != c.want {
			t.Errorf("IncrementNumber(%q, %v) = %q, want %q", c.in, c.delta, got, c.want)
		}
	}
}
