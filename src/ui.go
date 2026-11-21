package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

var vt bool

const (
	cReset  = "\x1b[0m"
	cBold   = "\x1b[1m"
	cDim    = "\x1b[90m"
	cGreen  = "\x1b[92m"
	cYellow = "\x1b[93m"
	cRed    = "\x1b[91m"
	cCyan   = "\x1b[96m"
	cGold   = "\x1b[33m"
)

func paint(c, s string) string {
	if !vt {
		return s
	}
	return c + s + cReset
}

func banner(version string) {
	line := strings.Repeat("=", 56)
	fmt.Println()
	fmt.Println(paint(cGold, "  "+line))
	fmt.Println(paint(cBold+cCyan, "     TACTICAL BREACH WIZARDS"))
	fmt.Println(paint(cBold, "     비공식 한국어 패치 ") + paint(cGreen, version))
	fmt.Println()
	fmt.Println("     번역 수정 · 패처  " + paint(cBold+cYellow, "LemoNade"))
	fmt.Println(paint(cDim, "     원본 번역         김하너 님과 지인분"))
	fmt.Println(paint(cDim, "     github.com/LemoNadeNade/TBW-Korean-Patch"))
	fmt.Println(paint(cGold, "  "+line))
	fmt.Println()
}

func okLine(s string)   { fmt.Println(paint(cGreen, "  [완료] ") + s) }
func warnLine(s string) { fmt.Println(paint(cYellow, "  [주의] ") + s) }
func errLine(s string)  { fmt.Println(paint(cRed, "  [오류] ") + s) }
func infoLine(s string) { fmt.Println("  " + s) }

// Flavour lines shown under the progress bar, by kind of work.
var flavour = map[string][]string{
	"scan":         {"작전 구역을 정찰하는 중...", "창문 위치를 하나하나 세는 중...", "엄폐물 뒤를 확인하는 중..."},
	"backup":       {"만약을 대비해 되감기 지점을 저장하는 중...", "원본을 녹색 구역으로 대피시키는 중..."},
	"font":         {"한글 글꼴을 케블라 조끼에 꿰매는 중...", "마법진에 한글 획을 그려 넣는 중...", "번개 마법으로 글꼴을 굽는 중..."},
	"story":        {"마법사들에게 한국어 발음을 가르치는 중...", "대사를 미리 예지해 두는 중...", "욕설 수위를 원작에 맞추는 중..."},
	"map":          {"작전 지도에 한국어 메모를 붙이는 중...", "임무 목표를 번역관에게 넘기는 중...", "창밖으로 던질 문장을 고르는 중..."},
	"ui":           {"버튼마다 한국어 이름표를 다는 중...", "특성 설명을 다듬는 중...", "턴 종료 버튼을 다시 칠하는 중..."},
	"link":         {"흩어진 글자를 한글 글꼴에 연결하는 중...", "글꼴 배선을 다시 납땜하는 중..."},
	"code":         {"되감기 버튼에 한국어를 새기는 중...", "게임 속 주문서를 고쳐 쓰는 중..."},
	"restore":      {"되감기 지점으로 돌아가는 중...", "원래 영어 대사를 꺼내 오는 중..."},
	"scan_done":    {"정찰 완료."},
	"done":         {"작전 준비 완료!"},
	"restore_done": {"원래 모습으로 돌아왔습니다."},
}

type progress struct {
	title     string
	total     int
	drawn     int
	lastDraw  time.Time
	lastKind  string
	flav      string
	flavSince time.Time
	start     time.Time
}

func newProgress(title string, total int) *progress {
	p := &progress{title: title, total: total, start: time.Now()}
	if vt {
		fmt.Print("\x1b[?25l") // hide cursor
	}
	return p
}

func (p *progress) pickFlavour(kind string) {
	if kind != p.lastKind || time.Since(p.flavSince) > 900*time.Millisecond {
		l := flavour[kind]
		if len(l) > 0 {
			p.flav = l[rand.Intn(len(l))]
		}
		p.lastKind = kind
		p.flavSince = time.Now()
	}
}

// update redraws the progress block. step describes the real work,
// kind selects the flavour line.
func (p *progress) update(done int, kind, step string, force bool) {
	if !vt {
		fmt.Printf("\r%s... %d/%d    ", p.title, done, p.total)
		return
	}
	if !force && time.Since(p.lastDraw) < 40*time.Millisecond {
		return
	}
	p.lastDraw = time.Now()
	p.pickFlavour(kind)
	const width = 20
	fill := 0
	if p.total > 0 {
		fill = done * width / p.total
	}
	pct := 0
	if p.total > 0 {
		pct = done * 100 / p.total
	}
	bar := paint(cGreen, strings.Repeat("■", fill)) + paint(cDim, strings.Repeat("□", width-fill))
	lines := []string{
		fmt.Sprintf("  %s  %s %s", paint(cBold, p.title), bar, paint(cBold, fmt.Sprintf("%3d%%", pct))),
		"  " + paint(cCyan, "▶ ") + step,
		"    " + paint(cDim, p.flav),
	}
	if p.drawn > 0 {
		fmt.Printf("\x1b[%dF", p.drawn)
	}
	for _, l := range lines {
		fmt.Print("\x1b[2K" + l + "\n")
	}
	p.drawn = len(lines)
}

func (p *progress) finish(kind, step string) {
	p.update(p.total, kind, step, true)
	if vt {
		fmt.Print("\x1b[?25h")
	} else {
		fmt.Println()
	}
}

func showCursor() {
	if vt {
		fmt.Print("\x1b[?25h")
	}
}

func elapsed(t time.Time) string {
	return fmt.Sprintf("%.1f초", time.Since(t).Seconds())
}
