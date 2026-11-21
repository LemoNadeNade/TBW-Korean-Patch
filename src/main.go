package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var in = bufio.NewReader(os.Stdin)

func readLine() string {
	s, _ := in.ReadString('\n')
	return strings.TrimSpace(s)
}

func ask(q string) bool {
	fmt.Print(q + " (Y/N): ")
	a := strings.ToLower(readLine())
	return a == "y" || a == "yes" || a == "ㅛ" || a == "예" || a == "네"
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "dev-roundtrip" {
		os.Exit(devRoundtrip(os.Args[2]))
	}
	if len(os.Args) >= 5 && os.Args[1] == "dev-dll" {
		os.Exit(devDLL(os.Args[2], os.Args[3], os.Args[4]))
	}
	vt = enableVT()
	var code int
	if demoBuild || (len(os.Args) >= 2 && os.Args[1] == "--demo") {
		code = runDemo()
		showCursor()
		os.Exit(code)
	}
	code = run()
	showCursor()
	if !batch {
		fmt.Println()
		fmt.Print(paint(cDim, "  Enter를 누르면 종료합니다."))
		readLine()
	}
	os.Exit(code)
}

var batch bool

func commas(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func run() int {
	var gameArg, pkgArg, action string
	yes := false
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--install":
			action = "install"
		case "--restore":
			action = "restore"
		case "--yes":
			yes = true
		case "--batch":
			batch, yes = true, true
		case "--plain":
			vt = false
		case "--pkg":
			if i+1 < len(args) {
				pkgArg = args[i+1]
				i++
			}
		default:
			gameArg = a
		}
	}

	var data []byte
	var err error
	if pkgArg != "" {
		data, err = os.ReadFile(pkgArg)
	} else {
		data, err = embeddedPackage()
	}
	if err != nil {
		errLine("패치 데이터를 읽지 못했습니다: " + err.Error())
		return 1
	}
	pkg, err := OpenPackage(data)
	if err != nil {
		errLine(err.Error())
		return 1
	}
	setTitle("Tactical Breach Wizards 한국어 패치 " + pkg.M.Version)
	banner(pkg.M.Version)

	root := ""
	if gameArg != "" {
		root = normalizeGameRoot(gameArg)
		if root == "" {
			errLine("지정한 경로에서 게임을 찾지 못했습니다: " + gameArg)
			return 1
		}
	} else {
		root = findGameRoot()
	}
	for root == "" {
		if batch {
			errLine("게임 폴더를 찾지 못했습니다.")
			return 1
		}
		warnLine("게임 폴더를 자동으로 찾지 못했습니다.")
		infoLine("Steam에서 게임 → 관리 → 로컬 파일 보기로 연 폴더를")
		infoLine("이 창에 끌어다 놓거나 경로를 입력한 뒤 Enter를 누르세요. (그냥 Enter: 종료)")
		fmt.Print("  > ")
		l := readLine()
		if l == "" {
			return 1
		}
		root = normalizeGameRoot(l)
		if root == "" {
			errLine("그 위치에서 게임 파일을 찾지 못했습니다.")
		}
	}
	infoLine(paint(cDim, "게임 폴더  ") + root)
	st := loadState(root)
	if len(st.Files) > 0 {
		infoLine(paint(cDim, "현재 상태  ") + paint(cGreen, "한국어 패치 적용됨") + paint(cDim, " ("+st.Package+")"))
	} else {
		infoLine(paint(cDim, "현재 상태  ") + "영어 (패치 안 됨)")
	}
	fmt.Println()

	if action == "" {
		infoLine(paint(cBold, "[1]") + " 한국어 패치 설치 / 업데이트")
		infoLine(paint(cBold, "[2]") + " 원래대로 되돌리기 (영어)")
		infoLine(paint(cBold, "[3]") + " 종료")
		fmt.Print("\n  > ")
		switch readLine() {
		case "1":
			action = "install"
		case "2":
			action = "restore"
		default:
			return 0
		}
		fmt.Println()
	}
	for gameRunning() {
		if batch {
			errLine("게임이 실행 중입니다. 게임을 끈 뒤 다시 실행하세요.")
			return 1
		}
		warnLine("게임이 실행 중이에요. 게임을 끈 뒤 Enter를 누르세요. (Q: 그만두기)")
		fmt.Print("  > ")
		if strings.EqualFold(readLine(), "q") {
			return 0
		}
	}
	switch action {
	case "install":
		err = install(root, pkg, yes)
	case "restore":
		err = restore(root, yes)
	}
	if err != nil {
		showCursor()
		fmt.Println()
		errLine(err.Error())
		if errors.Is(err, errInUse) {
			infoLine("게임이 실행 중이면 종료한 뒤 다시 실행하세요.")
		}
		return 1
	}
	return 0
}

var errInUse = errors.New("게임 파일을 쓸 수 없습니다")

type job struct {
	rel, target string
	spec        *FileSpec // nil for the DLL
	curSha      string
	fromBackup  bool
	known       bool
	kind, label string
}

var kindOrder = map[string]int{"font": 0, "story": 1, "map": 2, "ui": 3, "link": 4, "code": 5}

func classify(j *job) {
	if j.spec == nil {
		j.kind, j.label = "code", "게임 코드 문장 교체"
		return
	}
	hasBlob, hasSub, hasStr := false, false, false
	for i := range j.spec.Objects {
		o := &j.spec.Objects[i]
		hasBlob = hasBlob || o.Blob != ""
		hasSub = hasSub || len(o.Subs) > 0
		hasStr = hasStr || len(o.Strs) > 0
	}
	switch {
	case hasBlob:
		j.kind, j.label = "font", "한글 글꼴 설치, 대사·화면 번역"
	case hasSub:
		j.kind, j.label = "story", "대사 번역"
	case hasStr && strings.HasPrefix(j.rel, "level"):
		j.kind, j.label = "map", "임무 화면 번역"
	case hasStr:
		j.kind, j.label = "ui", "화면 문구 번역"
	default:
		j.kind, j.label = "link", "한글 글꼴 연결"
	}
}

// alreadyKorean reports whether a file that is not the known original was
// already rewritten by some Korean patch: a known output of the old patcher,
// a DLL full of Korean strings, or Korean font assets already in place.
func alreadyKorean(j *job, pkg *Package, h string) bool {
	for _, fh := range pkg.M.Foreign[j.rel] {
		if fh == h {
			return true
		}
	}
	if j.spec == nil {
		b, err := os.ReadFile(j.target)
		return err == nil && usHangulCount(b) > 200
	}
	hasBlob := false
	for i := range j.spec.Objects {
		hasBlob = hasBlob || j.spec.Objects[i].Blob != ""
	}
	if !hasBlob {
		return false
	}
	b, err := os.ReadFile(j.target)
	if err != nil {
		return false
	}
	f, err := ParseSFile(b)
	if err != nil {
		return false
	}
	for i := range j.spec.Objects {
		o := &j.spec.Objects[i]
		if o.Blob == "" {
			continue
		}
		if obj := f.Get(o.ID); obj != nil && sha(obj.Data) == o.Patched {
			return true
		}
	}
	return false
}

func checkWritable(p string) error {
	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("%w (%s)", errInUse, filepath.Base(p))
	}
	f.Close()
	return nil
}

func install(root string, pkg *Package, yes bool) error {
	data := filepath.Join(root, dataDirName)
	st := loadState(root)
	bdir := filepath.Join(root, backupDirName)
	t0 := time.Now()

	var jobs []*job
	for i := range pkg.M.Files {
		fs := &pkg.M.Files[i]
		jobs = append(jobs, &job{rel: fs.Path, target: filepath.Join(data, filepath.FromSlash(fs.Path)), spec: fs})
	}
	if pkg.M.DLL != nil {
		jobs = append(jobs, &job{rel: pkg.M.DLL.Path, target: filepath.Join(data, filepath.FromSlash(pkg.M.DLL.Path))})
	}
	for _, j := range jobs {
		classify(j)
	}
	sort.SliceStable(jobs, func(a, b int) bool { return kindOrder[jobs[a].kind] < kindOrder[jobs[b].kind] })

	unknown, foreign := 0, 0
	var total int64
	pr := newProgress("파일 확인", len(jobs))
	for n, j := range jobs {
		pr.update(n, "scan", "원본 파일 확인 "+paint(cDim, "("+filepath.Base(j.rel)+")"), false)
		fi, err := os.Stat(j.target)
		if err != nil {
			showCursor()
			return fmt.Errorf("게임 파일이 없습니다: %s", j.rel)
		}
		total += fi.Size()
		h, err := hashFile(j.target)
		if err != nil {
			showCursor()
			return err
		}
		j.curSha = h
		var vanilla string
		if j.spec != nil {
			vanilla = j.spec.Vanilla
		} else {
			vanilla = pkg.M.DLL.Vanilla
		}
		if fs, ok := st.Files[j.rel]; ok && fs.Patched == h {
			j.fromBackup = true
			j.known = fs.Original == vanilla
		} else {
			j.known = h == vanilla
		}
		if !j.known {
			unknown++
			if !j.fromBackup && alreadyKorean(j, pkg, h) {
				foreign++
			}
		}
	}
	pr.finish("scan_done", fmt.Sprintf("파일 %d개 확인 완료", len(jobs)))
	fmt.Println()
	if foreign > 0 {
		warnLine("다른 한국어 패치가 이미 적용된 상태입니다. (예: 예전 TBW_KR_Patcher)")
		infoLine("이 상태로 설치하면 원본 백업이 망가지므로 진행하지 않습니다.")
		infoLine("Steam에서 게임 → 속성 → 설치된 파일 → '게임 파일 무결성 확인'을 한 뒤 다시 실행하세요.")
		return nil
	}
	for _, j := range jobs {
		if err := checkWritable(j.target); err != nil {
			return err
		}
	}

	if unknown > 0 {
		warnLine(fmt.Sprintf("파일 %d개가 이 패치가 아는 원본과 다릅니다.", unknown))
		infoLine("- 다른 한국어 패치가 적용된 상태라면: Steam에서 게임 파일 무결성 확인을 한 뒤 다시 실행하세요.")
		infoLine("- 게임이 업데이트된 경우라면: 계속하면 맞는 부분만 적용하고, 나머지는 영어로 남습니다.")
		if !yes && !ask("  계속할까요?") {
			return nil
		}
	} else if !yes {
		newBackup := 0
		for _, j := range jobs {
			if !j.fromBackup {
				newBackup++
			}
		}
		if newBackup > 0 {
			infoLine(fmt.Sprintf("원본 파일 %d개(약 %d MB)를 %s 폴더에 백업한 뒤 패치합니다.", newBackup, total>>20, backupDirName))
		} else {
			infoLine("백업해 둔 원본에서 다시 패치합니다.")
		}
		if !ask("  진행할까요?") {
			return nil
		}
	}
	fmt.Println()

	st.Package = pkg.M.Name + " " + pkg.M.Version
	var sum struct{ applied, already, partial, skipped, missing, strMissed, strTotal int }
	var dllRes DLLResult
	pr = newProgress("패치", len(jobs))
	for n, j := range jobs {
		kind := j.kind
		backup := filepath.Join(bdir, filepath.FromSlash(j.rel))
		var src []byte
		var err error
		if j.fromBackup {
			pr.update(n, kind, j.label+" "+paint(cDim, "("+filepath.Base(j.rel)+")"), false)
			fs := st.Files[j.rel]
			if h, _ := hashFile(backup); h != fs.Original {
				showCursor()
				return fmt.Errorf("%w: %s", errBackupDamaged, j.rel)
			}
			src, err = os.ReadFile(backup)
		} else {
			pr.update(n, "backup", "원본 백업 "+paint(cDim, "("+filepath.Base(j.rel)+")"), false)
			src, err = os.ReadFile(j.target)
			if err == nil {
				if err = copyFileAtomic(j.target, backup); err == nil {
					st.Files[j.rel] = &fileState{Original: j.curSha}
					err = st.save()
				}
			}
			pr.update(n, kind, j.label+" "+paint(cDim, "("+filepath.Base(j.rel)+")"), false)
		}
		if err != nil {
			showCursor()
			return err
		}

		var out []byte
		if j.spec == nil {
			out, dllRes, err = PatchDLL(src, pkg.M.DLL.Replacements, pkg.M.DLL.MethodEdits)
			if err != nil {
				showCursor()
				return fmt.Errorf("%s: %w", j.rel, err)
			}
		} else {
			f, err := ParseSFile(src)
			if err != nil {
				showCursor()
				return fmt.Errorf("%s: %w", j.rel, err)
			}
			for k := range j.spec.Objects {
				ospec := &j.spec.Objects[k]
				obj := f.Get(ospec.ID)
				if obj == nil || obj.ClassID != ospec.Class {
					sum.missing++
					continue
				}
				var blob []byte
				if ospec.Blob != "" {
					if blob, err = pkg.Read(ospec.Blob); err != nil {
						showCursor()
						return err
					}
				}
				nd, r, s, err := PatchObject(obj.Data, ospec, blob)
				if err != nil {
					showCursor()
					return fmt.Errorf("%s #%d: %w", j.rel, ospec.ID, err)
				}
				obj.Data = nd
				sum.strTotal += s.strTotal
				sum.strMissed += s.strMissed
				switch r {
				case objApplied:
					sum.applied++
				case objAlready:
					sum.already++
				case objPartial:
					sum.partial++
				case objSkipped:
					sum.skipped++
				}
			}
			out = f.Bytes()
		}
		h := sha(out)
		if h != j.curSha {
			if err := writeAtomic(j.target, out); err != nil {
				showCursor()
				return fmt.Errorf("%w (%s): %v", errInUse, j.rel, err)
			}
		}
		st.Files[j.rel].Patched = h
		if err := st.save(); err != nil {
			showCursor()
			return err
		}
	}
	pr.finish("done", "모든 파일 패치 완료")
	fmt.Println()
	okLine(fmt.Sprintf("한국어 패치를 적용했습니다. %s", paint(cDim, "("+elapsed(t0)+")")))
	infoLine(fmt.Sprintf("       대사·화면 %s곳, 게임 코드 문장 %s곳", commas(sum.applied+sum.already), commas(dllRes.Replaced)))
	if sum.partial+sum.skipped+sum.missing > 0 || len(dllRes.Missing) > 0 {
		warnLine(fmt.Sprintf("%d곳은 게임 업데이트 등으로 원문이 달라 영어로 남았습니다.", sum.partial+sum.skipped+sum.missing+len(dllRes.Missing)))
	}
	infoLine(paint(cDim, "       원본 백업: "+bdir))
	for _, n := range pkg.M.Notes {
		infoLine("  * " + n)
	}
	fmt.Println()
	infoLine(paint(cCyan, "이제 게임을 켜면 한국어로 나와요. 즐거운 창문 돌파 되세요!"))
	return nil
}

func restore(root string, yes bool) error {
	data := filepath.Join(root, dataDirName)
	st := loadState(root)
	bdir := filepath.Join(root, backupDirName)
	if len(st.Files) == 0 {
		infoLine("이 프로그램으로 설치한 패치 기록이 없습니다.")
		infoLine("Steam에서 게임 파일 무결성 확인을 하면 원래 영어 상태로 돌아갑니다.")
		return nil
	}
	if !yes && !ask("  백업해 둔 원본으로 되돌릴까요?") {
		return nil
	}
	fmt.Println()
	t0 := time.Now()
	rels := make([]string, 0, len(st.Files))
	for r := range st.Files {
		rels = append(rels, r)
	}
	sort.Strings(rels)
	for _, r := range rels {
		if err := checkWritable(filepath.Join(data, filepath.FromSlash(r))); err != nil {
			return err
		}
	}
	restored, kept := 0, 0
	pr := newProgress("되돌리기", len(rels))
	for n, r := range rels {
		pr.update(n, "restore", "원본 복원 "+paint(cDim, "("+filepath.Base(r)+")"), false)
		fs := st.Files[r]
		target := filepath.Join(data, filepath.FromSlash(r))
		backup := filepath.Join(bdir, filepath.FromSlash(r))
		cur, err := hashFile(target)
		if err != nil {
			showCursor()
			return err
		}
		if cur == fs.Original {
			continue
		}
		if cur != fs.Patched {
			kept++ // replaced by a game update since; leave it alone
			continue
		}
		if h, _ := hashFile(backup); h != fs.Original {
			showCursor()
			return fmt.Errorf("%w: %s", errBackupDamaged, r)
		}
		if err := copyFileAtomic(backup, target); err != nil {
			showCursor()
			return fmt.Errorf("%w (%s): %v", errInUse, r, err)
		}
		restored++
	}
	pr.finish("restore_done", "모든 파일 복원 완료")
	fmt.Println()
	if err := os.RemoveAll(bdir); err != nil {
		warnLine("백업 폴더를 지우지 못했습니다. 직접 지워도 됩니다: " + bdir)
	}
	okLine(fmt.Sprintf("원래대로 되돌렸습니다. 파일 %d개 %s", restored, paint(cDim, "("+elapsed(t0)+")")))
	if kept > 0 {
		infoLine(fmt.Sprintf("파일 %d개는 그사이 게임 업데이트로 바뀌어 있어 그대로 두었습니다.", kept))
	}
	return nil
}
