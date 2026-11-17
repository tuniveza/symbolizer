package main

// ---- a notepad on the Design board (notes.go) ------------------------------------------------
//
// A notepad (kNotepad) shows notepad N's notes (From "notepad:N"): up to 16
// saved .txt files, oldest first.

const kNotepad = "notepad"

const notepadW, notepadH = 26, 16

func init() {
	kindIcons[kNotepad] = "▤"
	kindNames[kNotepad] = "notepad"
}
