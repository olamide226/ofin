package verify

import (
	"regexp"
	"strconv"
	"strings"
)

// Quantity is a numeric assertion extracted from text: (value, unit).
// Unit is normalized singular ("day", "week", "month", "year", "naira",
// "percent", "hour") or "" for a bare number.
type Quantity struct {
	Value float64
	Unit  string
}

var wordNumbers = map[string]float64{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11,
	"twelve": 12, "fourteen": 14, "fifteen": 15, "twenty": 20,
	"twenty-one": 21, "twenty-four": 24, "twenty-five": 25, "thirty": 30,
	"fifty": 50, "sixty": 60, "ninety": 90, "hundred": 100,
}

var unitAliases = map[string]string{
	"day": "day", "days": "day",
	"week": "week", "weeks": "week",
	"month": "month", "months": "month",
	"year": "year", "years": "year",
	"hour": "hour", "hours": "hour",
	"naira": "naira", "n": "naira", "₦": "naira",
	"percent": "percent", "%": "percent", "per cent": "percent",
	"kobo": "kobo",
}

var quantityRe = regexp.MustCompile(
	`(?i)(?:(₦|N)\s?)?(\d[\d,]*(?:\.\d+)?|` + wordAlternation() + `)` +
		`(?:\s*(?:working|clear|calendar|consecutive|full)?\s*(%|per cent|percent|days?|weeks?|months?|years?|hours?|naira|kobo))?`)

func wordAlternation() string {
	words := make([]string, 0, len(wordNumbers))
	for w := range wordNumbers {
		words = append(words, w)
	}
	// Longer alternatives first so "twenty-four" beats "twenty".
	for i := 0; i < len(words); i++ {
		for j := i + 1; j < len(words); j++ {
			if len(words[j]) > len(words[i]) {
				words[i], words[j] = words[j], words[i]
			}
		}
	}
	return `\b(?:` + strings.Join(words, "|") + `)\b`
}

// ExtractQuantities pulls (value, unit) pairs from text. Bare numbers under
// 32 with no unit are ignored — they are usually list markers or section
// references that survived stripping, and they generate false mismatches.
func ExtractQuantities(text string) []Quantity {
	var out []Quantity
	for _, m := range quantityRe.FindAllStringSubmatch(text, -1) {
		currency, numStr, unitStr := m[1], m[2], m[3]
		var val float64
		if v, ok := wordNumbers[strings.ToLower(numStr)]; ok {
			val = v
		} else {
			v, err := strconv.ParseFloat(strings.ReplaceAll(numStr, ",", ""), 64)
			if err != nil {
				continue
			}
			val = v
		}
		unit := ""
		if currency != "" {
			unit = "naira"
		}
		if unitStr != "" {
			if u, ok := unitAliases[strings.ToLower(unitStr)]; ok {
				unit = u
			}
		}
		if unit == "" && val < 32 {
			continue
		}
		// Bare four-digit numbers in the statute-year range are act names
		// ("Employees' Compensation Act 2010"), not quantities.
		if unit == "" && val >= 1900 && val <= 2100 && val == float64(int(val)) {
			continue
		}
		out = append(out, Quantity{Value: val, Unit: unit})
	}
	return out
}

// UnsupportedQuantities returns the claim quantities that appear neither in
// the cited source nor in the user's question. Question quantities are the
// user's own situation ("I worked 3 years", "I earn 450k") restated by a
// good answer — only quantities the MODEL introduced must be grounded in
// the statutory text. Matching: value must agree; units match when equal or
// when either side is unit-less.
func UnsupportedQuantities(claim, source, question string) []Quantity {
	known := append(ExtractQuantities(source), ExtractQuantities(question)...)
	var missing []Quantity
	for _, cq := range ExtractQuantities(claim) {
		found := false
		for _, sq := range known {
			if cq.Value == sq.Value && (cq.Unit == sq.Unit || cq.Unit == "" || sq.Unit == "") {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, cq)
		}
	}
	return missing
}
