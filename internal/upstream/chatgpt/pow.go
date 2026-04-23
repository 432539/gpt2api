// Package chatgpt — POW 算法迁移自 gen_image.py
//
// chatgpt.com sentinel 使用两种 POW token:
//
//   1. RequirementsToken  → 客户端主动生成,塞进 /sentinel/chat-requirements
//      请求体的 `p` 字段。前缀 "gAAAAAC"。固定难度 "0fffff"。
//      config 是 18 元素数组,迭代 config[3] 与 config[9]。
//
//   2. ProofToken         → 服务端返回 `proofofwork.required=true` + seed + difficulty,
//      客户端本地求解后放进 Header `openai-sentinel-proof-token`。
//      前缀 "gAAAAAB"。config 是 13 元素数组,只迭代 config[3]。
//
// 两者共享同一个判定函数:SHA3-512(seed + base64(config_json)) 的前 N 字节
// 按字节序 <= bytes.fromhex(difficulty)。若不满足则 config[3] += 1 重试。
package chatgpt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/sha3"
)

// TurnstileSolver 负责把 /sentinel/chat-requirements/prepare 返回的 `turnstile.dx`
// 挑战字符串,解算成 /sentinel/chat-requirements/finalize 需要的 turnstile
// response 字符串。
//
// 说明:OpenAI 的 turnstile 是基于 Cloudflare turnstile 衍生的自定义 challenge,
// dx 是混淆 JS + WebAssembly 的输入,response 是执行结果。纯 Go 无法还原,
// 解算必须委托给外部服务(2captcha/capsolver/自建 headless 浏览器等)。
//
// 没有 solver 时,Client.ChatRequirementsV2 会自动回退到老的单步
// chat-requirements 流程(Turnstile=true 直接忽略)。
type TurnstileSolver interface {
	Solve(ctx context.Context, dx string, token string) (string, error)
}

const (
	powPrefixRequirements = "gAAAAAC"
	powPrefixProof        = "gAAAAAB"

	requirementsDifficulty = "0fffff"

	maxRequirementsIter = 500_000
	maxProofIter        = 100_000

	powFallback = "gAAAAABwQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D"
)

var (
	powCores   = []int{16, 24, 32}
	powScreens = []int{3000, 4000, 6000}

	powNavKeys = []string{
		"webdriver−false", "vendor−Google Inc.", "cookieEnabled−true",
		"pdfViewerEnabled−true", "hardwareConcurrency−32",
		"language−zh-CN", "mimeTypes−[object MimeTypeArray]",
		"userAgentData−[object NavigatorUAData]",
	}
	powWinKeys = []string{
		"innerWidth", "innerHeight", "devicePixelRatio", "screen",
		"chrome", "location", "history", "navigator",
	}

	powReactListeners = []string{"_reactListeningcfilawjnerp", "_reactListening9ne2dfo1i47"}
	powProofEvents    = []string{"alert", "ontransitionend", "onprogress"}

	// perfCounter 模拟浏览器 performance.counter() 的单调递增(亚秒级)。
	perfCounter uint64
)

// POWConfig 是 18 元素的客户端指纹数组(requirements_token 用)。
type POWConfig struct {
	userAgent string
	arr       [18]interface{}
}

// NewPOWConfig 构造一个随机化的客户端指纹,用于 requirements + proof 两种场景。
func NewPOWConfig(userAgent string) *POWConfig {
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	//nolint:gosec // 非加密用途
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	now := time.Now().UTC()
	timeStr := now.Format("Mon Jan 02 2006 15:04:05") + " GMT+0000 (UTC)"
	perf := float64(atomic.AddUint64(&perfCounter, 1)) + rng.Float64()

	c := &POWConfig{userAgent: userAgent}
	c.arr = [18]interface{}{
		powCores[rng.Intn(len(powCores))] + powScreens[rng.Intn(len(powScreens))], // 0
		timeStr,                 // 1
		nil,                     // 2
		rng.Float64(),           // 3 - 迭代会覆盖
		userAgent,               // 4
		nil,                     // 5
		"dpl=1440a687921de39ff5ee56b92807faaadce73f13", // 6
		"en-US",             // 7
		"en-US,zh-CN",       // 8
		0,                   // 9 - 迭代会覆盖
		powNavKeys[rng.Intn(len(powNavKeys))], // 10
		"location",          // 11
		powWinKeys[rng.Intn(len(powWinKeys))], // 12
		perf,                // 13
		randomUUID(rng),     // 14
		"",                  // 15
		8,                   // 16
		now.Unix(),          // 17
	}
	return c
}

// RequirementsToken 生成 /sentinel/chat-requirements 的 "p" 字段值。
// 对齐 gen_image.py.get_requirements_token:固定难度 0fffff,前缀 gAAAAAC。
func (c *POWConfig) RequirementsToken() string {
	//nolint:gosec
	seed := strconv.FormatFloat(rand.Float64(), 'f', -1, 64)
	b64, ok := c.solveRequirements(seed, requirementsDifficulty)
	if !ok {
		return powPrefixRequirements + powFallback +
			base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	}
	return powPrefixRequirements + b64
}

// solveRequirements 高性能迭代:预拼 JSON 的三段字节前缀,只在内循环拼 d1/d2。
// 严格对齐 gen_image.py._generate_answer。
func (c *POWConfig) solveRequirements(seed, difficulty string) (string, bool) {
	target, err := hex.DecodeString(difficulty)
	if err != nil {
		return "", false
	}
	diffLen := len(difficulty) // 字符数(与 Python 对齐)

	// 预拼 p1/p2/p3。config[3] 和 config[9] 位置留给迭代器。
	arr := c.arr
	// p1 = json(arr[:3])[:-1] + ","
	head, _ := json.Marshal([]interface{}{arr[0], arr[1], arr[2]})
	p1 := append(head[:len(head)-1:len(head)-1], ',')

	mid, _ := json.Marshal([]interface{}{arr[4], arr[5], arr[6], arr[7], arr[8]})
	// p2 = "," + json(arr[4:9])[1:-1] + ","
	p2 := make([]byte, 0, len(mid)+2)
	p2 = append(p2, ',')
	p2 = append(p2, mid[1:len(mid)-1]...)
	p2 = append(p2, ',')

	tail, _ := json.Marshal([]interface{}{
		arr[10], arr[11], arr[12], arr[13], arr[14], arr[15], arr[16], arr[17],
	})
	// p3 = "," + json(arr[10:])[1:]  => "," + "element1,...,elementN]"
	p3 := make([]byte, 0, len(tail)+1)
	p3 = append(p3, ',')
	p3 = append(p3, tail[1:]...)

	hasher := sha3.New512()
	seedB := []byte(seed)
	buf := make([]byte, 0, len(p1)+32+len(p2)+16+len(p3))
	b64buf := make([]byte, base64.StdEncoding.EncodedLen(cap(buf)))

	for i := 0; i < maxRequirementsIter; i++ {
		d1 := strconv.Itoa(i)
		d2 := strconv.Itoa(i >> 1)

		buf = buf[:0]
		buf = append(buf, p1...)
		buf = append(buf, d1...)
		buf = append(buf, p2...)
		buf = append(buf, d2...)
		buf = append(buf, p3...)

		n := base64.StdEncoding.EncodedLen(len(buf))
		if cap(b64buf) < n {
			b64buf = make([]byte, n)
		}
		b64buf = b64buf[:n]
		base64.StdEncoding.Encode(b64buf, buf)

		hasher.Reset()
		hasher.Write(seedB)
		hasher.Write(b64buf)
		sum := hasher.Sum(nil)

		// Python: h[:diff_len] <= target
		// diff_len 是字符数(6),target 是字节(3)。Python bytes cmp 按短的逐字节比较。
		// 这里保持等价:取 min(len(target), len(sum)) 字节比较。
		n2 := diffLen
		if n2 > len(sum) {
			n2 = len(sum)
		}
		cmpLen := n2
		if cmpLen > len(target) {
			cmpLen = len(target)
		}
		if bytes.Compare(sum[:cmpLen], target[:cmpLen]) <= 0 {
			return string(b64buf), true
		}
	}
	return "", false
}

// SolveProofToken 按服务端挑战求解 proof token(header 用,前缀 gAAAAAB)。
// 迁移自 gen_image.py.generate_proof_token 的轻量 13 元素 config。
func SolveProofToken(seed, difficulty, userAgent string) string {
	if seed == "" || difficulty == "" {
		return ""
	}
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	//nolint:gosec
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	screen := powScreens[rng.Intn(len(powScreens))] * (1 << rng.Intn(3)) // *1/2/4

	timeStr := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

	proofConfig := []interface{}{
		screen, // 0
		timeStr,
		nil,
		0, // 3 - 迭代
		userAgent,
		"https://tcr9i.chat.openai.com/v2/35536E1E-65B4-4D96-9D97-6ADB7EFF8147/api.js",
		"dpl=1440a687921de39ff5ee56b92807faaadce73f13",
		"en",
		"en-US",
		nil,
		"plugins−[object PluginArray]",
		powReactListeners[rng.Intn(len(powReactListeners))],
		powProofEvents[rng.Intn(len(powProofEvents))],
	}

	diffLen := len(difficulty)
	hasher := sha3.New512()
	for i := 0; i < maxProofIter; i++ {
		proofConfig[3] = i
		raw, err := json.Marshal(proofConfig)
		if err != nil {
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(raw)
		hasher.Reset()
		hasher.Write([]byte(seed + b64))
		sum := hasher.Sum(nil)
		hexStr := hex.EncodeToString(sum)
		if strings.Compare(hexStr[:diffLen], difficulty) <= 0 {
			return powPrefixProof + b64
		}
	}
	return powPrefixProof + powFallback +
		base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
}

func randomUUID(rng *rand.Rand) string {
	var b [16]byte
	_, _ = rng.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
