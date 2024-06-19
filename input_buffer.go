package prompt

import "bytes"

type BufferParser struct {
	Col    uint16
	Row    uint16
	buffer *bytes.Buffer
}

func (p *BufferParser) Setup() error {
	buf := make([]byte, 1024)
	p.buffer = bytes.NewBuffer(buf)
	return nil
}
func (p *BufferParser) TearDown() error {
	return nil
}
func (p *BufferParser) TearDownDisableEcho() error {
	return nil
}
func (p *BufferParser) GetWinSize() *WinSize {
	return &WinSize{
		Row: p.Row,
		Col: p.Col,
	}
}
func (t *BufferParser) SetWinSize(winsize *WinSize) {

}

func (p *BufferParser) Read() ([]byte, error) {
	buf := make([]byte, maxReadBytes)
	n, err := p.buffer.Read(buf)
	if err != nil {
		return []byte{}, err
	}
	return buf[:n], nil
}
func NewBufferInputParser(col, row uint16) *BufferParser {
	return &BufferParser{
		Col: col,
		Row: row,
	}
}
