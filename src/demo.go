package main

import (
	"fmt"
	"math/rand"
	"time"
)

// Preview mode: shows every screen of the patcher with simulated work.
// It never opens game files for writing.

const demoVersion = "v1.0.1"

func sleepMs(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

func demoJobs() []*job {
	var js []*job
	js = append(js, &job{rel: "sharedassets0.assets", kind: "font", label: "한글 글꼴 설치, 대사·화면 번역"})
	for i := 0; i <= 128; i++ {
		if i%9 == 7 {
			continue
		}
		js = append(js, &job{rel: fmt.Sprintf("level%d", i), kind: "map", label: "임무 화면 번역"})
	}
	for i := 1; i <= 128; i++ {
		k, l := "link", "한글 글꼴 연결"
		if i%11 == 3 {
			k, l = "ui", "화면 문구 번역"
		}
		js = append(js, &job{rel: fmt.Sprintf("sharedassets%d.assets", i), kind: k, label: l})
	}
	js = append(js, &job{rel: "Managed/Assembly-CSharp.dll", kind: "code", label: "게임 코드 문장 교체"})
	return js[:234]
}

func demoHeader(root string, patched bool) {
	banner(demoVersion)
	fmt.Println(paint(cYellow, "  [미리보기] 화면만 보여 주는 데모예요. 게임 파일은 건드리지 않아요."))
	fmt.Println()
	infoLine(paint(cDim, "게임 폴더  ") + root)
	if patched {
		infoLine(paint(cDim, "현재 상태  ") + paint(cGreen, "한국어 패치 적용됨") + paint(cDim, " (Tactical Breach Wizards 비공식 한국어 패치 "+demoVersion+")"))
	} else {
		infoLine(paint(cDim, "현재 상태  ") + "영어 (패치 안 됨)")
	}
	fmt.Println()
}

func clearScreen() {
	if vt {
		fmt.Print("\x1b[2J\x1b[H")
	}
}

func runDemo() int {
	root := findGameRoot()
	if root == "" {
		root = `G:\Steam\steamapps\common\Tactical Breach Wizards (예시)`
	}
	setTitle("Tactical Breach Wizards 한국어 패치 " + demoVersion + " (미리보기)")
	patched := false
	for {
		clearScreen()
		demoHeader(root, patched)
		infoLine(paint(cBold, "[1]") + " 설치 화면 보기")
		infoLine(paint(cBold, "[2]") + " 업데이트 화면 보기 (이미 설치된 상태에서)")
		infoLine(paint(cBold, "[3]") + " 되돌리기 화면 보기")
		infoLine(paint(cBold, "[4]") + " 경고·오류 화면 보기")
		infoLine(paint(cBold, "[5]") + " 종료")
		fmt.Print("\n  > ")
		switch readLine() {
		case "1":
			fmt.Println()
			demoInstall(false)
			patched = true
		case "2":
			fmt.Println()
			demoInstall(true)
			patched = true
		case "3":
			fmt.Println()
			demoRestore()
			patched = false
		case "4":
			fmt.Println()
			demoWarnings()
		default:
			return 0
		}
		showCursor()
		fmt.Println()
		fmt.Print(paint(cDim, "  Enter를 누르면 메뉴로 돌아갑니다."))
		readLine()
	}
}

func demoInstall(update bool) {
	jobs := demoJobs()
	t0 := time.Now()
	pr := newProgress("파일 확인", len(jobs))
	for n, j := range jobs {
		pr.update(n, "scan", "원본 파일 확인 "+paint(cDim, "("+j.rel+")"), false)
		if j.rel == "sharedassets0.assets" {
			sleepMs(400)
		} else {
			sleepMs(8)
		}
	}
	pr.finish("scan_done", fmt.Sprintf("파일 %d개 확인 완료", len(jobs)))
	fmt.Println()
	if update {
		infoLine("백업해 둔 원본에서 다시 패치합니다.")
	} else {
		infoLine(fmt.Sprintf("원본 파일 %d개(약 480 MB)를 %s 폴더에 백업한 뒤 패치합니다.", len(jobs), backupDirName))
	}
	if !ask("  진행할까요?") {
		return
	}
	fmt.Println()
	t0 = time.Now()
	pr = newProgress("패치", len(jobs))
	for n, j := range jobs {
		if !update {
			pr.update(n, "backup", "원본 백업 "+paint(cDim, "("+j.rel+")"), false)
			sleepMs(6)
		}
		pr.update(n, j.kind, j.label+" "+paint(cDim, "("+j.rel+")"), false)
		switch j.kind {
		case "font":
			sleepMs(1800)
		case "code":
			sleepMs(900)
		case "map", "ui":
			sleepMs(22 + rand.Intn(10))
		default:
			sleepMs(10)
		}
	}
	pr.finish("done", "모든 파일 패치 완료")
	fmt.Println()
	okLine(fmt.Sprintf("한국어 패치를 적용했습니다. %s", paint(cDim, "("+elapsed(t0)+")")))
	infoLine("       대사·화면 11,400곳, 게임 코드 문장 730곳")
	infoLine(paint(cDim, `       원본 백업: G:\Steam\steamapps\common\Tactical Breach Wizards\TBW_KR_Backup`))
	fmt.Println()
	infoLine(paint(cCyan, "이제 게임을 켜면 한국어로 나와요. 즐거운 창문 돌파 되세요!"))
}

func demoRestore() {
	if !ask("  백업해 둔 원본으로 되돌릴까요?") {
		return
	}
	fmt.Println()
	jobs := demoJobs()
	t0 := time.Now()
	pr := newProgress("되돌리기", len(jobs))
	for n, j := range jobs {
		pr.update(n, "restore", "원본 복원 "+paint(cDim, "("+j.rel+")"), false)
		if j.kind == "font" {
			sleepMs(700)
		} else {
			sleepMs(12)
		}
	}
	pr.finish("restore_done", "모든 파일 복원 완료")
	fmt.Println()
	okLine(fmt.Sprintf("원래대로 되돌렸습니다. 파일 %d개 %s", len(jobs), paint(cDim, "("+elapsed(t0)+")")))
}

func demoWarnings() {
	fmt.Println(paint(cBold, "  ── 게임이 켜져 있을 때 ──"))
	warnLine("게임이 실행 중이에요. 게임을 끈 뒤 Enter를 누르세요. (Q: 그만두기)")
	fmt.Print("  > ")
	readLine()
	fmt.Println()
	fmt.Println(paint(cBold, "  ── 다른 한국어 패치가 적용된 상태일 때 ──"))
	warnLine("다른 한국어 패치가 이미 적용된 상태입니다. (예: 예전 TBW_KR_Patcher)")
	infoLine("이 상태로 설치하면 원본 백업이 망가지므로 진행하지 않습니다.")
	infoLine("Steam에서 게임 → 속성 → 설치된 파일 → '게임 파일 무결성 확인'을 한 뒤 다시 실행하세요.")
	fmt.Println()
	fmt.Println(paint(cBold, "  ── 게임이 업데이트되어 원본이 달라졌을 때 ──"))
	warnLine("파일 3개가 이 패치가 아는 원본과 다릅니다.")
	infoLine("- 다른 한국어 패치가 적용된 상태라면: Steam에서 게임 파일 무결성 확인을 한 뒤 다시 실행하세요.")
	infoLine("- 게임이 업데이트된 경우라면: 계속하면 맞는 부분만 적용하고, 나머지는 영어로 남습니다.")
	fmt.Println()
	okLine("한국어 패치를 적용했습니다. " + paint(cDim, "(5.8초)"))
	infoLine("       대사·화면 11,388곳, 게임 코드 문장 730곳")
	warnLine("12곳은 게임 업데이트 등으로 원문이 달라 영어로 남았습니다.")
	fmt.Println()
	fmt.Println(paint(cBold, "  ── 게임 폴더를 못 찾았을 때 ──"))
	warnLine("게임 폴더를 자동으로 찾지 못했습니다.")
	infoLine("Steam에서 게임 → 관리 → 로컬 파일 보기로 연 폴더를")
	infoLine("이 창에 끌어다 놓거나 경로를 입력한 뒤 Enter를 누르세요. (그냥 Enter: 종료)")
	fmt.Println()
	fmt.Println(paint(cBold, "  ── 파일을 쓸 수 없을 때 ──"))
	errLine("게임 파일을 쓸 수 없습니다 (sharedassets0.assets)")
	infoLine("게임이 실행 중이면 종료한 뒤 다시 실행하세요.")
}
