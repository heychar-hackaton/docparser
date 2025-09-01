package extract

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// ExtractText detects file type by extension and extracts plain text.
func ExtractText(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return extractPDF(data)
	case ".docx":
		return extractDOCX(data)
	case ".rtf":
		return extractRTF(data)
	case ".txt", "":
		return extractTXT(data)
	default:
		// Try best-effort: docx are zips, pdf start with %PDF, rtf starts with {\rtf
		if bytes.HasPrefix(data, []byte("%PDF")) {
			return extractPDF(data)
		}
		if bytes.HasPrefix(data, []byte("PK")) {
			return extractDOCX(data)
		}
		if bytes.HasPrefix(data, []byte("{\\rtf")) {
			return extractRTF(data)
		}
		return "", errors.New("unsupported file type: " + ext)
	}
}

func extractPDF(data []byte) (string, error) {
	cmd := exec.Command("pdftotext", "-layout", "-", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	if _, err := stdin.Write(data); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		return "", err
	}
	_ = stdin.Close()
	out, err := io.ReadAll(stdout)
	if err != nil {
		_ = cmd.Wait()
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}
	return string(out), nil
}

func extractDOCX(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", errors.New("document.xml not found in docx")
	}
	rc, err := docFile.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	dec := xml.NewDecoder(rc)
	var b strings.Builder
	type element struct{ space, local string }
	var stack []element

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, element{space: t.Name.Space, local: t.Name.Local})
			switch t.Name.Local {
			case "br":
				b.WriteByte('\n')
			case "tab":
				b.WriteByte('\t')
			case "t":
				// read text until end of this element
				var txt strings.Builder
				for {
					tok2, err2 := dec.Token()
					if err2 == io.EOF {
						break
					}
					if err2 != nil {
						return "", err2
					}
					if char, ok := tok2.(xml.CharData); ok {
						txt.WriteString(string(char))
						continue
					}
					if end, ok := tok2.(xml.EndElement); ok && end.Name.Local == "t" {
						break
					}
				}
				b.WriteString(txt.String())
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			switch t.Name.Local {
			case "p":
				b.WriteByte('\n')
			}
		}
	}
	return b.String(), nil
}

func extractRTF(data []byte) (string, error) {
	// Minimal, best-effort RTF to text converter
	var b strings.Builder
	depth := 0
	skipUntilDepth := -1

	isLetter := func(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
	i := 0
	for i < len(data) {
		c := data[i]
		switch c {
		case '{':
			depth++
			i++
			continue
		case '}':
			if skipUntilDepth >= 0 && depth == skipUntilDepth {
				skipUntilDepth = -1
			}
			if depth > 0 {
				depth--
			}
			i++
			continue
		case '\\':
			i++
			if i >= len(data) {
				break
			}
			// control symbol like \\', \{, \}
			if !isLetter(data[i]) {
				sym := data[i]
				i++
				switch sym {
				case '\\', '{', '}':
					if skipUntilDepth < 0 {
						b.WriteByte(sym)
					}
				case '~':
					if skipUntilDepth < 0 {
						b.WriteByte(' ')
					}
				case '-':
					// optional hyphen – ignore
				case '_':
					// non-breaking hyphen – write '-'
					if skipUntilDepth < 0 {
						b.WriteByte('-')
					}
				case '*':
					// destination control – skip next group
					if skipUntilDepth < 0 {
						skipUntilDepth = depth
					}
				case '\'':
					// hex encoded byte: \'hh
					if i+1 < len(data) {
						hh := data[i : i+2]
						i += 2
						var dst [1]byte
						if _, err := hex.Decode(dst[:], hh); err == nil {
							if skipUntilDepth < 0 {
								b.WriteByte(dst[0])
							}
						}
					}
				default:
					// ignore other symbols
				}
				continue
			}
			// control word
			start := i
			for i < len(data) && isLetter(data[i]) {
				i++
			}
			word := string(data[start:i])
			// optional numeric argument (can be negative)
			neg := false
			if i < len(data) && (data[i] == '-' || (data[i] >= '0' && data[i] <= '9')) {
				if data[i] == '-' {
					neg = true
					i++
				}
				numStart := i
				for i < len(data) && data[i] >= '0' && data[i] <= '9' {
					i++
				}
				numStr := string(data[numStart:i])
				if word == "u" {
					if v, err := strconv.Atoi(numStr); err == nil {
						if neg {
							v = -v
						}
						if skipUntilDepth < 0 {
							b.WriteRune(rune(int32(v)))
						}
						// skip optional replacement char
						if i < len(data) && data[i] == '?' {
							i++
						}
					}
				}
			}
			// control words with direct effects
			switch word {
			case "par", "line":
				if skipUntilDepth < 0 {
					b.WriteByte('\n')
				}
			case "tab":
				if skipUntilDepth < 0 {
					b.WriteByte('\t')
				}
			case "fonttbl", "colortbl", "stylesheet", "info", "pict", "header", "footer":
				if skipUntilDepth < 0 {
					skipUntilDepth = depth
				}
			}
			// a control word may end with space, which should be swallowed
			if i < len(data) && data[i] == ' ' {
				i++
			}
			continue
		default:
			// In RTF, raw CR/LF are formatting-only; ignore them and rely on \par/\line
			if c == '\r' || c == '\n' {
				i++
				continue
			}
			if skipUntilDepth < 0 {
				b.WriteByte(c)
			}
			i++
		}
	}
	// ensure valid UTF-8; if not, try to interpret as UTF-16 LE/BE with BOM
	out := b.String()
	// Normalize whitespace: unify newlines, collapse multiples, remove spaces before punctuation
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")
	reNewlines := regexp.MustCompile(`\n{2,}`)
	out = reNewlines.ReplaceAllString(out, "\n")
	reSpaces := regexp.MustCompile(`[ \t]{2,}`)
	out = reSpaces.ReplaceAllString(out, " ")
	reSpaceBeforePunct := regexp.MustCompile(`\s+([,.:;!?])`)
	out = reSpaceBeforePunct.ReplaceAllString(out, "$1")
	if !utf8.ValidString(out) {
		// try decode as UTF-16 with BOM
		bs := []byte(out)
		if len(bs) >= 2 {
			if bs[0] == 0xFF && bs[1] == 0xFE { // LE
				u := make([]uint16, 0, (len(bs)-2)/2)
				for j := 2; j+1 < len(bs); j += 2 {
					u = append(u, uint16(bs[j])|uint16(bs[j+1])<<8)
				}
				runes := utf16.Decode(u)
				return string(runes), nil
			}
			if bs[0] == 0xFE && bs[1] == 0xFF { // BE
				u := make([]uint16, 0, (len(bs)-2)/2)
				for j := 2; j+1 < len(bs); j += 2 {
					u = append(u, uint16(bs[j+1])|uint16(bs[j])<<8)
				}
				runes := utf16.Decode(u)
				return string(runes), nil
			}
		}
	}
	return out, nil
}

func extractTXT(data []byte) (string, error) {
	// Handle UTF-16 BOMs
	if len(data) >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE { // UTF-16 LE
			u := make([]uint16, 0, (len(data)-2)/2)
			for i := 2; i+1 < len(data); i += 2 {
				u = append(u, uint16(data[i])|uint16(data[i+1])<<8)
			}
			s := string(utf16.Decode(u))
			s = strings.ReplaceAll(s, "\r\n", "\n")
			s = strings.ReplaceAll(s, "\r", "\n")
			return s, nil
		}
		if data[0] == 0xFE && data[1] == 0xFF { // UTF-16 BE
			u := make([]uint16, 0, (len(data)-2)/2)
			for i := 2; i+1 < len(data); i += 2 {
				u = append(u, uint16(data[i+1])|uint16(data[i])<<8)
			}
			s := string(utf16.Decode(u))
			s = strings.ReplaceAll(s, "\r\n", "\n")
			s = strings.ReplaceAll(s, "\r", "\n")
			return s, nil
		}
	}
	if utf8.Valid(data) {
		s := string(data)
		s = strings.ReplaceAll(s, "\r\n", "\n")
		s = strings.ReplaceAll(s, "\r", "\n")
		return s, nil
	}
	// Try common Cyrillic encodings and pick the best match
	if decoded, ok := decodeBestCyrillic(data); ok {
		decoded = strings.ReplaceAll(decoded, "\r\n", "\n")
		decoded = strings.ReplaceAll(decoded, "\r", "\n")
		return decoded, nil
	}
	// Fallback: ISO-8859-1 mapping
	runes := make([]rune, 0, len(data))
	for _, by := range data {
		runes = append(runes, rune(by))
	}
	s := string(runes)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s, nil
}

// decodeBestCyrillic tries a list of common Cyrillic encodings and returns the best-scoring text.
func decodeBestCyrillic(data []byte) (string, bool) {
	candidates := []struct {
		name string
		enc  *charmap.Charmap
	}{
		{"windows-1251", charmap.Windows1251},
		{"koi8-r", charmap.KOI8R},
		{"iso-8859-5", charmap.ISO8859_5},
		{"mac-cyrillic", charmap.MacintoshCyrillic},
		{"cp866", charmap.CodePage866},
	}
	bestText := ""
	bestScore := int(-1 << 31)

	for _, c := range candidates {
		r := transform.NewReader(bytes.NewReader(data), c.enc.NewDecoder())
		decoded, err := io.ReadAll(r)
		if err != nil {
			continue
		}
		text := string(decoded)
		score := scoreCyrillicText(text)
		if score > bestScore {
			bestScore = score
			bestText = text
		}
	}
	if bestText == "" {
		return "", false
	}
	// Heuristic: require some Cyrillic or at least no replacement chars
	if strings.ContainsRune(bestText, '\uFFFD') {
		return "", false
	}
	return bestText, true
}

func scoreCyrillicText(s string) int {
	if s == "" {
		return -1_000_000
	}
	var cyr, asciiPrint, repl, ctrl int
	for _, r := range s {
		switch {
		case r == '\uFFFD':
			repl++
		case r >= 0x0400 && r <= 0x052F: // Cyrillic + Extended
			cyr++
		case r >= 0x20 && r <= 0x7E: // ASCII printable
			asciiPrint++
		case r < 0x20 && r != '\n' && r != '\t' && r != '\r':
			ctrl++
		}
	}
	// Penalize replacement and control chars, reward Cyrillic
	return 3*cyr + asciiPrint - 50*repl - 5*ctrl
}
