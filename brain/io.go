package brain

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

//go:embed weights.bin
var embedded []byte

// Default loads the weights embedded in the binary.
func Default() (*Model, error) { return Load(embedded) }

// Format: "PUFI", C L NH T, moods, characters, then every tensor in groups of
// 64 values: a float32 scale + int8 values. This makes the weights 4x smaller than float32,
// and a scale per group keeps rare large weights from eating the precision of the rest.

const group = 64

func (m *Model) Save(w io.Writer) error {
	le := binary.LittleEndian
	buf := &bytes.Buffer{}
	buf.WriteString("PUFI")
	for _, v := range []int{m.C, m.L, m.NH, m.T, len(m.Moods)} {
		_ = binary.Write(buf, le, int32(v))
	}
	for _, md := range m.Moods {
		buf.WriteByte(byte(len(md)))
		buf.WriteString(md)
	}
	_ = binary.Write(buf, le, int32(len(m.Chars)))
	for _, c := range m.Chars {
		_ = binary.Write(buf, le, int32(c))
	}
	for _, p := range m.params() {
		for g := 0; g < len(p); g += group {
			chunk := p[g:min(g+group, len(p))]
			var mx float32
			for _, v := range chunk {
				mx = max(mx, float32(math.Abs(float64(v))))
			}
			scale := mx / 127
			if scale == 0 {
				scale = 1
			}
			_ = binary.Write(buf, le, scale)
			q := make([]int8, len(chunk))
			for i, v := range chunk {
				q[i] = int8(math.Round(float64(v / scale)))
			}
			_ = binary.Write(buf, le, q)
		}
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func Load(data []byte) (*Model, error) {
	le := binary.LittleEndian
	r := bytes.NewReader(data)
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != "PUFI" {
		return nil, errors.New("brain: unknown weights format")
	}
	var hdr [5]int32
	if err := binary.Read(r, le, &hdr); err != nil {
		return nil, err
	}
	moods := make([]string, hdr[4])
	for i := range moods {
		l, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		b := make([]byte, l)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, err
		}
		moods[i] = string(b)
	}
	var nc int32
	if err := binary.Read(r, le, &nc); err != nil {
		return nil, err
	}
	raw := make([]int32, nc)
	if err := binary.Read(r, le, raw); err != nil {
		return nil, err
	}
	chars := make([]rune, nc)
	for i, c := range raw {
		chars[i] = rune(c)
	}
	m := newModel(Config{C: int(hdr[0]), L: int(hdr[1]), NH: int(hdr[2]), T: int(hdr[3])}, moods, chars)
	for _, p := range m.params() {
		for g := 0; g < len(p); g += group {
			chunk := p[g:min(g+group, len(p))]
			var scale float32
			if err := binary.Read(r, le, &scale); err != nil {
				return nil, err
			}
			q := make([]int8, len(chunk))
			if err := binary.Read(r, le, q); err != nil {
				return nil, err
			}
			for i, v := range q {
				chunk[i] = float32(v) * scale
			}
		}
	}
	return m, nil
}
