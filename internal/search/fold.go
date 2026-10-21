package search

import (
	"strings"
	"unicode"
)

// Fold normalizes text for matching: lower-case, full-width ASCII to
// half-width, the ideographic space to a plain space, and katakana to
// hiragana, so "ファン" matches "ふぁん" and "ＧＰＵ" matches "gpu". Half-width
// katakana is left alone. It maps rune for rune, so a folded string has the
// same rune count as its input and match positions carry back to the original.
func Fold(s string) string {
	return strings.Map(foldRune, s)
}

func foldRune(r rune) rune {
	switch {
	case r >= 0x30A1 && r <= 0x30F6: // katakana ァ..ヶ -> hiragana ぁ..ゖ
		r -= 0x60
	case r >= 0xFF01 && r <= 0xFF5E: // full-width ！..～ -> ASCII
		r -= 0xFEE0
	case r == 0x3000: // ideographic space
		r = ' '
	}
	return unicode.ToLower(r)
}
