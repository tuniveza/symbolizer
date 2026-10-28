package main

import "testing"

func TestGuessLanguage(t *testing.T) {
	cases := []struct{ text, ext string }{
		{"#include <iostream>\nusing namespace std;\nint main() {\n    cout << \"hi\" << endl;\n}\n", ".cpp"},
		{"#include <vector>\nint main() {\n    std::vector<int> v;\n    return 0;\n}\n", ".cpp"},
		{"#include <stdio.h>\nint main(void) {\n    printf(\"hi\\n\");\n    return 0;\n}\n", ".c"},
		{"#include <stdlib.h>\nstruct node { int v; };\nint main() { struct node *n = malloc(sizeof *n); free(n); }\n", ".c"},
		{"package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n", ".go"},
		{"fn main() {\n    let mut x = 1;\n    println!(\"{}\", x);\n}\n", ".rs"},
		{"def greet(name):\n    print('hi', name)\n\nif __name__ == '__main__':\n    greet('x')\n", ".py"},
		{"import os\nfor f in os.listdir('.'):\n    print(f)\n", ".py"},
		{"const x = require('fs');\nfunction go() {\n  console.log('hi');\n}\n", ".js"},
		{"interface User {\n  name: string;\n  age: number;\n}\nconst u: User = { name: 'a', age: 1 };\n", ".ts"},
		{"public class Hello {\n    public static void main(String[] args) {\n        System.out.println(\"hi\");\n    }\n}\n", ".java"},
		{"using System;\nclass P { static void Main() { Console.WriteLine(\"hi\"); } }\n", ".cs"},
		{"#!/bin/bash\necho hi\n", ".sh"},
		{"for f in *; do\n  echo \"$f\"\ndone\n", ".sh"},
		{"local x = 1\nif x ~= 2 then\n  print(x)\nend\n", ".lua"},
		{"<!DOCTYPE html>\n<html><body><p>hi</p></body></html>\n", ".html"},
		{"body {\n  margin: 0;\n  padding: 10px;\n}\n", ".css"},
		{"{\"a\": 1, \"b\": [1, 2]}", ".json"},
		{"# Title\n\nSome **bold** text and a [link](http://x).\n", ".md"},
		{"SELECT name FROM users WHERE id = 1;\n", ".sql"},
		{"just some words I am writing down\n", ".txt"},
		{"", ".txt"},
	}
	for _, c := range cases {
		name, ext := guessLanguage(c.text)
		if ext != c.ext {
			t.Errorf("guessed %s (%s) for %q, want %s", ext, name, c.text, c.ext)
		}
	}
}
