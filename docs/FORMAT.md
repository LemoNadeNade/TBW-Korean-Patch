# 패키지 형식 (tbwkr-1)

패처에 들어가는 번역 데이터(`.tbkr`)는 zip 파일입니다. 게임 파일을 통째로 담지 않고,
**원본 게임 파일에 적용할 변경점**만 담습니다. 예외는 무료 글꼴로 만든 한국어 글꼴 데이터입니다.

```
manifest.json          변경점 목록
payload/*.bin          한국어 글꼴 에셋과 글꼴 아틀라스 텍스처
```

## manifest.json

```jsonc
{
  "format": "tbwkr-1",
  "name": "Tactical Breach Wizards 비공식 한국어 패치",
  "version": "v1.0.1",
  "files": [
    {
      "path": "level37",                 // Tactical Breach Wizards_Data 기준 경로
      "vanilla_sha256": "…",             // 지원하는 원본 파일의 해시
      "objects": [
        {
          "id": 1234,                    // Unity 오브젝트 path ID
          "class": 114,                  // Unity class ID
          "v": "…", "p": "…",            // 원본 / 패치 후 오브젝트 해시
          "s": [[offset, "원문", "번역"]],          // 문자열 필드 교체
          "t": [{"off": offset, "edits": [["원문 조각", "번역 조각"]]}],  // 긴 문자열(대사 스크립트) 안의 부분 교체
          "b": [[offset, "원래 바이트(hex)", "새 바이트(hex)"]],         // 같은 길이 바이트 교체 (글꼴 연결)
          "blob": "payload/…"            // 통째 교체 (글꼴 에셋만)
        }
      ]
    }
  ],
  "dll": {
    "path": "Managed/Assembly-CSharp.dll",
    "vanilla_sha256": "…",
    "replacements": [["원문", "번역"]],     // 모든 ldstr 문자열 교체
    "method_edits": [ … ]                  // 특정 메서드 안에서만 바꾸는 문장
  },
  "foreign": { "경로": ["예전 패처 결과 해시", …] }
}
```

## 적용 방식

- 오브젝트 해시가 원본(`v`)과 같으면 기록된 위치에 그대로 적용하고, 결과 해시(`p`)로 검증합니다.
- 해시가 다르면(게임 업데이트 등) 원문 문자열을 찾아 맞는 것만 적용하고 나머지는 건너뜁니다.
- DLL은 새 문자열을 `#US` 힙 뒤에 추가하고 `ldstr` 토큰만 바꾼 뒤, 메타데이터를 마지막 섹션 뒤로 옮깁니다.
- 처음 설치할 때 원본을 `TBW_KR_Backup`에 백업하고, 이후 업데이트는 항상 백업한 원본에서 다시 적용합니다.
