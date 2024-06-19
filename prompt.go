package prompt

import (
	"bytes"
	"fmt"
	"os"
	"time"
	"unicode/utf8"

	"github.com/RainyBow/go-prompt/internal/debug"
)

var (
	sleep_interval time.Duration = 10 // Millisecond
)

// Executor is called when user input something text.
type Executor func(string)

// ExitChecker is called after user input to check if prompt must stop and exit go-prompt Run loop.
// User input means: selecting/typing an entry, then, if said entry content matches the ExitChecker function criteria:
// - immediate exit (if breakline is false) without executor called
// - exit after typing <return> (meaning breakline is true), and the executor is called first, before exit.
// Exit means exit go-prompt (not the overall Go program)
type ExitChecker func(in string, breakline bool) bool

// Completer should return the suggest item from Document.
type Completer func(Document) []Suggest

// Prompt is core struct of go-prompt.
type Prompt struct {
	in                ConsoleParser
	buf               *Buffer
	renderer          *Render
	executor          Executor
	history           *History
	completion        *CompletionManager
	keyBindings       []KeyBind
	ASCIICodeBindings []ASCIICodeBind
	keyBindMode       KeyBindMode
	completionOnDown  bool
	exitChecker       ExitChecker
	skipTearDown      bool
	recordHistory     bool
}

// Exec is the struct contains user input context.
type Exec struct {
	input string
}

// turn off history record
func (p *Prompt) StopRecordHistory() {
	p.recordHistory = false
}

// turn on history record
func (p *Prompt) StartRecordHistory() {
	p.recordHistory = true
}

// turn on echo
func (p *Prompt) StartEcho() {
	// 开启回显，当命令行exec函数需要命令行输入或者进入shell模式时，调用该函数
	p.in.TearDown()
}

// turn off echo
func (p *Prompt) StopEcho() {
	// 关闭回显
	p.in.TearDownDisableEcho()
}

// set history max size
func (p *Prompt) SetMaxHistory(max int) {
	p.history.UpdateMax(max)
}

// Run starts prompt.
func (p *Prompt) Run() {
	p.skipTearDown = false
	defer debug.Teardown()
	debug.Log("start prompt")
	p.setUp()
	defer p.tearDown()

	if p.completion.showAtStart {
		p.completion.Update(*p.buf.Document())
	}

	p.renderer.Render(p.buf, p.completion)

	bufCh := make(chan []byte, 2048000)
	stopReadBufCh := make(chan struct{})
	go p.readBuffer(bufCh, stopReadBufCh)

	exitCh := make(chan int)
	winSizeCh := make(chan *WinSize)
	stopHandleSignalCh := make(chan struct{})
	go p.handleSignals(exitCh, winSizeCh, stopHandleSignalCh)
	tmp_bytes := []byte{}
	for {
		select {
		case b := <-bufCh:
			debug.Log(fmt.Sprintf("b from bufCh,bytes:%v", b))
			debug.Log(fmt.Sprintf("b from bufCh,string:%s", b))
			tmp_bytes = append(tmp_bytes, b...)
			debug.Log(fmt.Sprintf("tmp_bytes,string:%s", tmp_bytes))
			lines, rest := splitBytes(tmp_bytes)
			if len(tmp_bytes) > 1 && len(lines) > 0 {
				debug.Log(fmt.Sprintf("lines splitBytes,raw:%v", lines))
				debug.Log(fmt.Sprintf("rest splitBytes,raw:%v,", rest))
				debug.Log(fmt.Sprintf("rest splitBytes,string:%s,", rest))
				tmp_bytes = rest
				for _, line := range lines {
					p.feedbytes(line, stopReadBufCh, stopHandleSignalCh, bufCh, exitCh, winSizeCh)
					p.feedbytes([]byte{0xa}, stopReadBufCh, stopHandleSignalCh, bufCh, exitCh, winSizeCh)
				}
				// } else {
				// 	if utf8.Valid(tmp_bytes) {
				// 		p.feedbytes(tmp_bytes, stopReadBufCh, stopHandleSignalCh, bufCh, exitCh, winSizeCh)
				// 		tmp_bytes = []byte{}
				// 	}
				// 	// p.feedbytes(tmp_bytes, stopReadBufCh, stopHandleSignalCh, bufCh, exitCh, winSizeCh)
				// 	// tmp_bytes = []byte{}
				// }
			}
			if utf8.Valid(tmp_bytes) {
				residue := []byte{}
				tmp_bytes, residue = separateCompleteAndOther(tmp_bytes, residue) //需要接收返回值，因为该函数主要修改tmp和residue的len，对底层数组改动较小
				p.feedbytes(tmp_bytes, stopReadBufCh, stopHandleSignalCh, bufCh, exitCh, winSizeCh)
				tmp_bytes = residue
			}

		case w := <-winSizeCh:
			p.renderer.UpdateWinSize(w)
			p.renderer.Render(p.buf, p.completion)
		case code := <-exitCh:
			p.renderer.BreakLine(p.buf)
			p.tearDown()
			os.Exit(code)
		}
	}
}

// 区分完整命令和不完整命令，不完整的命令暂不参与处理
func separateCompleteAndOther(tmp_bytes, residue []byte) ([]byte, []byte) {
	escLastIndex := bytes.LastIndex(tmp_bytes, []byte{0x1b})
	if escLastIndex != -1 {
		complete := false
		residue = tmp_bytes[escLastIndex:]
		for _, k := range ASCIISequences {
			if k.Key == Escape && bytes.Equal(k.ASCIICode, residue) {
				break
			}
			if bytes.Equal(k.ASCIICode, residue) {
				complete = true
				break
			}
		}
		if !complete && (len(residue) == 1 || residue[1] == 0x5b) { // 命令不完整且符合ansi控制码前缀规则的，仅保存至缓存区
			tmp_bytes = tmp_bytes[:escLastIndex]
		} else { // 命令完整，或不符合ansi前缀规则的，正常执行
			residue = []byte{}
		}
	}
	return tmp_bytes, residue
}
func (p *Prompt) feedbytes(byts []byte, stopReadBufCh, stopHandleSignalCh chan struct{},
	bufCh chan []byte, exitCh chan int, winSizeCh chan *WinSize) {
	if shouldExit, e := p.feed(byts); shouldExit {
		debug.Log("feedbytes, shouldExit")
		p.renderer.BreakLine(p.buf)
		stopReadBufCh <- struct{}{}
		stopHandleSignalCh <- struct{}{}
		return
	} else if e != nil {
		// Stop goroutine to run readBuffer function
		debug.Log(fmt.Sprintf("feedbytes, executor:%s", e.input))
		stopReadBufCh <- struct{}{}
		stopHandleSignalCh <- struct{}{}

		// Unset raw mode
		// Reset to Blocking mode because returned EAGAIN when still set non-blocking mode.
		debug.AssertNoError(p.in.TearDownDisableEcho())
		p.executor(e.input)

		p.completion.Update(*p.buf.Document())

		p.renderer.Render(p.buf, p.completion)

		if p.exitChecker != nil && p.exitChecker(e.input, true) {
			p.skipTearDown = true
			return
		}
		// Set raw mode
		debug.AssertNoError(p.in.Setup())
		go p.readBuffer(bufCh, stopReadBufCh)
		go p.handleSignals(exitCh, winSizeCh, stopHandleSignalCh)
	} else {
		p.completion.Update(*p.buf.Document())
		p.renderer.Render(p.buf, p.completion)
	}
}

func (p *Prompt) BreakLine() {
	p.renderer.BreakLine(p.buf)
}

func (p *Prompt) feed(b []byte) (shouldExit bool, exec *Exec) {
	key, bytes := GetKey(b)

	p.buf.lastKeyStroke = key
	// completion
	completing := p.completion.Completing()
	p.handleCompletionKeyBinding(key, completing)
	debug.Log(fmt.Sprintf("feed key:%s", key))
	debug.Log(fmt.Sprintf("feed b:%s", b))
	debug.Log(fmt.Sprintf("feed buf:%s", p.buf.Text()))
	switch key {
	case Enter, ControlJ, ControlM:
		p.renderer.BreakLine(p.buf)

		exec = &Exec{input: p.buf.Text()}
		p.buf.Close()
		p.buf = NewBuffer()
		if exec.input != "" && p.recordHistory {
			p.history.Add(exec.input)
		}
	case ControlC:
		p.renderer.BreakLine(p.buf)
		p.buf.Close()
		p.buf = NewBuffer()
		p.history.Clear()
	case Up, ControlP:
		if !completing { // Don't use p.completion.Completing() because it takes double operation when switch to selected=-1.
			if newBuf, changed := p.history.Older(p.buf); changed {
				p.buf.Close()
				p.buf = newBuf
			}
		}
	case Down, ControlN:
		if !completing { // Don't use p.completion.Completing() because it takes double operation when switch to selected=-1.
			if newBuf, changed := p.history.Newer(p.buf); changed {
				p.buf.Close()
				p.buf = newBuf
			}
			return
		}
	// case ControlD:
	// 	if p.buf.Text() == "" {
	// 		shouldExit = true
	// 		return
	// 	}
	case NotDefined:
		if p.handleASCIICodeBinding(bytes) {
			return
		}
		p.buf.InsertText(string(bytes), false, true)
	}

	shouldExit = p.handleKeyBinding(key)
	return
}

func (p *Prompt) handleCompletionKeyBinding(key Key, completing bool) {
	switch key {
	case Down:
		if completing || p.completionOnDown {
			p.completion.Next()
		}
	case Tab, ControlI:
		p.completion.Next()
	case Up:
		if completing {
			p.completion.Previous()
		}
	case BackTab:
		p.completion.Previous()
	default:
		if s, ok := p.completion.GetSelectedSuggestion(); ok {
			w := p.buf.Document().GetWordBeforeCursorUntilSeparator(p.completion.wordSeparator)
			if w != "" {
				p.buf.DeleteBeforeCursor(len([]rune(w)))
			}
			p.buf.InsertText(s.Text, false, true)
		}
		p.completion.Reset()
	}
}

func (p *Prompt) handleKeyBinding(key Key) bool {
	shouldExit := false
	for i := range commonKeyBindings {
		kb := commonKeyBindings[i]
		if kb.Key == key {
			kb.Fn(p.buf)
		}
	}

	if p.keyBindMode == EmacsKeyBind {
		for i := range emacsKeyBindings {
			kb := emacsKeyBindings[i]
			if kb.Key == key {
				kb.Fn(p.buf)
			}
		}
	}

	// Custom key bindings
	for i := range p.keyBindings {
		kb := p.keyBindings[i]
		if kb.Key == key {
			kb.Fn(p.buf)
		}
	}
	if p.exitChecker != nil && p.exitChecker(p.buf.Text(), false) {
		shouldExit = true
	}
	return shouldExit
}

// register key binding
func (p *Prompt) RegisterKeyBinding(kb KeyBind) error {
	for _, kbind := range p.keyBindings {
		if kbind.Key == kb.Key {
			return fmt.Errorf("Key(%s) has bind function", kb.Key.String())
		}
	}
	p.keyBindings = append(p.keyBindings, kb)
	return nil
}

// unregister key binding
func (p *Prompt) UnregisterKeyBinding(key Key) error {
	found := false
	keyBindings := []KeyBind{}
	for _, kbind := range p.keyBindings {
		if kbind.Key == key {
			found = true
		} else {
			keyBindings = append(keyBindings, kbind)
		}
	}
	if found {
		p.keyBindings = keyBindings
		return nil
	}
	return fmt.Errorf("Key(%s) has not binding to function", key.String())
}

func (p *Prompt) handleASCIICodeBinding(b []byte) bool {
	checked := false
	for _, kb := range p.ASCIICodeBindings {
		if bytes.Equal(kb.ASCIICode, b) {
			kb.Fn(p.buf)
			checked = true
		}
	}
	return checked
}

func (p *Prompt) ClearOutput() {
	p.renderer.out.EraseEndOfLine()
}

// Input just returns user input text.
func (p *Prompt) Input() string {
	defer debug.Teardown()
	debug.Log("start prompt")
	p.setUp()
	defer p.tearDown()

	if p.completion.showAtStart {
		p.completion.Update(*p.buf.Document())
	}

	p.renderer.Render(p.buf, p.completion)
	bufCh := make(chan []byte, maxReadBytes)
	stopReadBufCh := make(chan struct{})
	go p.readBuffer(bufCh, stopReadBufCh)

	for b := range bufCh {
		if shouldExit, e := p.feed(b); shouldExit {
			p.renderer.BreakLine(p.buf)
			stopReadBufCh <- struct{}{}
			return ""
		} else if e != nil {
			// Stop goroutine to run readBuffer function
			stopReadBufCh <- struct{}{}
			return e.input
		} else {
			p.completion.Update(*p.buf.Document())
			p.renderer.Render(p.buf, p.completion)
		}

	}
	return ""

}

// Get input Window Size
func (p *Prompt) GetWinSize() *WinSize {
	return p.in.GetWinSize()
}

// Set input Window Size
func (p *Prompt) SetWindowSize(winsize *WinSize) {
	p.renderer.UpdateWinSize(winsize)
	p.in.SetWinSize(winsize)
}
func (p *Prompt) readBuffer(bufCh chan []byte, stopCh chan struct{}) {
	debug.Log("start reading buffer")
	for {
		select {
		case <-stopCh:
			debug.Log("stop reading buffer")
			return
		default:
			if b, err := p.in.Read(); err == nil && !(len(b) == 1 && b[0] == 0) {
				bufCh <- b
			}
		}
		time.Sleep(sleep_interval * time.Millisecond)
	}
}

func (p *Prompt) setUp() {
	debug.AssertNoError(p.in.Setup())
	p.renderer.Setup()
	p.renderer.UpdateWinSize(p.in.GetWinSize())
}

func (p *Prompt) tearDown() {
	if !p.skipTearDown {
		debug.AssertNoError(p.in.TearDown())
	}
	p.renderer.TearDown()
}

var (
	lineSeps = [][]byte{{'\r', '\n'}, {'\n', '\r'}, {'\r'}}
	lineSep  = []byte{'\n'}
)

func splitBytes(input []byte) (list [][]byte, rest []byte) {
	list = [][]byte{input}
	if len(input) == 1 {
		return
	}
	for _, sep := range lineSeps {
		if bytes.Contains(input, sep) {
			input = bytes.ReplaceAll(input, sep, lineSep)
		}
	}
	list = bytes.Split(input, lineSep)    // list 长度一定>=1
	if !bytes.HasSuffix(input, lineSep) { // list 长度一定>=2
		rest = list[len(list)-1]
	}
	list = list[:len(list)-1]
	return

}
