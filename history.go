package prompt

// History stores the texts that are entered.
type History struct {
	histories []string
	tmp       []string
	selected  int
	max       int // 0 means unlimit
}

// update max history size
func (h *History) UpdateMax(max int) {
	if len(h.histories) <= max {
		h.max = max
		return
	}
	h.histories = h.histories[len(h.histories)-max:]
	h.Clear()

}

// Add to add text in history.
func (h *History) Add(input string) {
	if len(h.histories) > 0 && input == h.histories[len(h.histories)-1] {
		// 和上一条重复的忽略
		h.Clear()
		return
	}
	if h.max > 0 && len(h.histories) == h.max {
		h.histories = append(h.histories[1:], input)
	} else {
		h.histories = append(h.histories, input)
	}
	h.Clear()
}

// Clear to clear the history.
func (h *History) Clear() {
	h.tmp = make([]string, len(h.histories)+1)
	copy(h.tmp, h.histories)
	h.selected = len(h.histories)

}

// Older saves a buffer of current line and get a buffer of previous line by up-arrow.
// The changes of line buffers are stored until new history is created.
func (h *History) Older(buf *Buffer) (new *Buffer, changed bool) {
	if len(h.tmp) == 1 || h.selected == 0 {
		return buf, false
	}
	h.tmp[h.selected] = buf.Text()

	h.selected--
	new = NewBuffer()
	new.InsertText(h.tmp[h.selected], false, true)
	return new, true
}

// Newer saves a buffer of current line and get a buffer of next line by up-arrow.
// The changes of line buffers are stored until new history is created.
func (h *History) Newer(buf *Buffer) (new *Buffer, changed bool) {
	if h.selected >= len(h.tmp)-1 {
		return buf, false
	}
	h.tmp[h.selected] = buf.Text()

	h.selected++
	new = NewBuffer()
	new.InsertText(h.tmp[h.selected], false, true)
	return new, true
}

// NewHistory returns new history object.
func NewHistory() *History {
	return &History{
		histories: []string{},
		tmp:       []string{""},
		selected:  0,
	}
}
