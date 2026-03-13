# Prompt Guard v3.1.0 — Token Optimization Release

> **550+ 공격 패턴 · 11 SHIELD 카테고리 · 10개 언어 지원**
> 
> 보안 성능 100% 유지하면서 토큰 소모량 최대 90% 절감

---

## 🛡️ 현재 Prompt Guard의 보안 규모

| 지표 | 수치 | 의미 |
|------|------|------|
| **총 공격 패턴** | 550+ | 직접 주입부터 MCP 악용까지 |
| **SHIELD 카테고리** | 11개 | prompt, tool, mcp, memory, supply_chain 등 |
| **지원 언어** | 10개 | EN, KO, JA, ZH, RU, ES, DE, FR, PT, VI |
| **인코딩 탐지** | 6종 | Base64, Hex, ROT13, URL, HTML, Unicode |
| **테스트 커버리지** | 115개 | 모든 기능 회귀 테스트 |

**v1.0 (50개 패턴) → v3.1 (550+ 패턴): 11배 성장**

---

## ⚡ v3.1.0의 핵심: 왜 토큰 최적화인가?

### 문제 인식

AI 에이전트의 컨텍스트 윈도우는 유한합니다. SKILL.md가 크면:
- 🔴 대화 길이 제한 (컨텍스트 소진)
- 🔴 응답 지연 (토큰 처리 시간)
- 🔴 비용 증가 (토큰당 과금)

기존 Prompt Guard는 744줄의 SKILL.md로 **매 세션 ~5-6k 토큰**을 소비했습니다.

### 해결 방법

**"보안은 그대로, 토큰만 줄이자"**

| 최적화 | 절감률 | 원리 |
|--------|--------|------|
| SKILL.md 경량화 | 65% | 문서 분리, Quick Start만 유지 |
| 티어드 패턴 로딩 | 70% | 필요한 패턴만 점진적 로드 |
| 메시지 해시 캐시 | 90% | 중복 분석 제거 |

---

## 🆕 v3.1.0 새 기능 상세

### 1. 티어드 패턴 로딩 (Tiered Pattern Loading)

**컨셉:** 모든 위협이 동등하지 않다. 진짜 위험한 건 항상 체크하고, 나머지는 필요할 때만.

| Tier | 패턴 | 언제? | 예시 |
|------|------|-------|------|
| **Tier 0** | CRITICAL ~30개 | 항상 | API키 탈취, `rm -rf`, SQL injection |
| **Tier 1** | HIGH ~70개 | 기본 | 인스트럭션 오버라이드, 탈옥 시도 |
| **Tier 2** | MEDIUM ~100개 | 위협 시 | 역할 조작, 감정 조작 |

**보안 유지 원리:**
```
일반 메시지 → Tier 0+1 스캔 (100개) → 안전 → 통과
의심 메시지 → Tier 0+1 스캔 → 위협 감지 → Tier 2 확장 → 전체 550+ 스캔
```

위협이 감지되면 즉시 전체 패턴으로 확장합니다. **안전할 땐 빠르게, 위험할 땐 철저하게.**

### 2. 메시지 해시 캐시 (Message Hash Cache)

**컨셉:** 같은 메시지는 같은 결과. 두 번 분석할 필요 없다.

| 설정 | 값 |
|------|-----|
| 캐시 크기 | LRU 1,000개 |
| 해시 알고리즘 | SHA-256 |
| 스레드 안전 | ✅ |

**보안 유지 원리:**
- 메시지 원문 저장 안 함 (해시만)
- 동일 입력 = 동일 출력 (결정적 함수)
- 새 메시지는 항상 전체 분석

### 3. 패턴 외부화 (External Pattern Files)

**컨셉:** SKILL.md에서 패턴을 분리하여 컨텍스트 토큰 절약.

```
patterns/
├── critical.yaml   # Tier 0 (~30개)
├── high.yaml       # Tier 1 (~70개)
└── medium.yaml     # Tier 2 (~100개)
```

패턴은 Python 런타임에서 YAML로 로드됩니다. **탐지 로직은 동일, 컨텍스트 부담만 감소.**

---

## 📊 실제 절감 효과

### 일반 대화 (대부분의 경우)
```
이전: 744줄 SKILL.md 로드 → ~5-6k 토큰
이후: 261줄 SKILL.md 로드 → ~1.5-2k 토큰
절감: 65-70%
```

### 반복 메시지 (인사, 자주 쓰는 표현)
```
이전: 매번 전체 분석
이후: 캐시 히트 → 즉시 반환
절감: 90%+
```

### 위협 탐지 시
```
Tier 확장 → 전체 550+ 패턴 로드
보안 성능: 100% 유지
```

---

## 📜 최근 10개 릴리즈 히스토리

### v3.1.0 (2026-02-09) — 이번 릴리즈 ⭐
토큰 최적화: 티어드 로딩, 해시 캐시, SKILL.md 경량화

### v3.0.1 (2026-02-08)
HiveFence Scout Round 3: Sockpuppetting, TrojanPraise, Promptware Kill Chain

### v3.0.0 (2026-02-07) — 메이저 릴리즈
패키지 리팩토링: `scripts/detect.py` → `prompt_guard/` 모듈화

### v2.8.2 (2026-02-07)
토큰 스플리팅 방어 100% 커버리지, 한국어 데이터 탈취 패턴 11개 추가

### v2.8.1 (2026-02-07)
Enterprise DLP: `sanitize_output()` 자격증명 자동 수정, 17개 크리덴셜 포맷

### v2.8.0 (2026-02-07)
Phase 1 하드닝: 6종 인코딩 디코드, 출력 DLP, 카나리 토큰, 76개 테스트

### v2.7.0 (2026-02-05)
HiveFence Scout: MCP 악용, 유니코드 태그, 브라우저 에이전트 공격 등 6개 카테고리

### v2.6.2 (2026-02-05)
10개 언어 확장: 러시아어, 스페인어, 독일어, 프랑스어, 포르투갈어, 베트남어

### v2.6.1 (2026-02-05)
HiveFence Scout: Allowlist 우회, Hooks 하이재킹, 서브에이전트 악용 등 5개 카테고리

### v2.6.0 (2026-02-01)
실전 레드팀 대응: 단일 승인 확장, 크리덴셜 경로 하베스팅, DM 소셜 엔지니어링

---

## 🔢 버전별 성장 추이

| 버전 | 패턴 수 | 주요 추가 |
|------|---------|----------|
| v1.0 | 50+ | 기본 프롬프트 주입 방어 |
| v2.0 | 130+ | 다국어, 심각도 점수 |
| v2.5 | 349 | 7개 신규 카테고리 |
| v2.6 | 400+ | 10개 언어, 소셜 엔지니어링 |
| v2.7 | 460+ | MCP/브라우저 에이전트 |
| v2.8 | 500+ | 인코딩 우회, DLP |
| v3.0 | 520+ | 패키지 리팩토링, SHIELD.md |
| **v3.1** | **550+** | **토큰 최적화** |

---

## 🚀 시작하기

### 설치
```bash
pip install prompt-guard
# 또는
clawdhub install prompt-guard
```

### 사용
```python
from prompt_guard import PromptGuard

guard = PromptGuard()
result = guard.analyze("user message")

if result.action == "block":
    return "🚫 차단됨"
```

### 설정 (선택)
```yaml
prompt_guard:
  pattern_tier: high   # critical, high, full
  cache:
    enabled: true
    max_size: 1000
```

---

## 📎 링크

- **GitHub:** [seojoonkim/prompt-guard](https://github.com/seojoonkim/prompt-guard)
- **ClawdHub:** [clawdhub.com/skills/prompt-guard](https://clawdhub.com/skills/prompt-guard)
- **HiveFence:** [hivefence.com](https://hivefence.com)

---

**Author:** Seojoon Kim  
**License:** MIT  
**Date:** 2026-02-09
