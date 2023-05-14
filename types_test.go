package types_splitter_plugin

import "testing"

func Test_findClosingBracket1(t *testing.T) {
	type args struct {
		openBracket rune
		input       string
		start       int
		excOpen     bool
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "Simple test case with no nested brackets",
			args: args{
				openBracket: '(',
				input:       "(hello)",
				start:       0,
				excOpen:     false,
			},
			want: 6,
		},
		{
			name: "Test case with nested brackets",
			args: args{
				openBracket: '(',
				input:       "((hello))",
				start:       0,
				excOpen:     false,
			},
			want: 8,
		},
		{
			name: "Test case with no matching closing bracket",
			args: args{
				openBracket: '(',
				input:       "(hello",
				start:       0,
				excOpen:     false,
			},
			want: len("(hello"),
		},
		{
			name: "Test case with quoted brackets that should be ignored",
			args: args{
				openBracket: '(',
				input:       `"(hello (world))"`,
				start:       0,
				excOpen:     false,
			},
			want: len(`"(hello (world))"`),
		},
		{
			name: "Test case starting from an offset",
			args: args{
				openBracket: '(',
				input:       "((hello) (world))",
				start:       1,
				excOpen:     false,
			},
			want: 7,
		},
		{
			name: "Test case function",
			args: args{
				openBracket: '(',
				input: `func hello(name string) string {
return "hello (name)
}"`,
				start:   32,
				excOpen: false,
			},
			want: 56,
		},
		{
			name: "Test case function with start before bracket",
			args: args{
				openBracket: '{',
				input: `func hello(name string) string {
return "hello (name)
}"`,
				start:   10,
				excOpen: false,
			},
			want: 56,
		},
		{
			name: "Test case function with start after bracket",
			args: args{
				openBracket: '{',
				input: `func hello(name string) string {
return "hello (name)
}"`,
				start:   38,
				excOpen: true,
			},
			want: 56,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := findClosingBracket(tt.args.openBracket, tt.args.input, tt.args.start, tt.args.excOpen); got != tt.want {
				t.Errorf("findClosingBracket() = %v, want %v", got, tt.want)
			}
		})
	}
}
