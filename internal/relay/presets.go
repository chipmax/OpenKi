package relay

import (
	"regexp"
	"strings"

	"zen-gate/internal/store"
)

// Preset is a curated free (or free-tier) upstream the dashboard offers as a
// one-click fill for the 自定义 API form. The user still supplies their own
// key — nothing here embeds credentials. Endpoints verified against the
// providers' docs and the community-maintained list
// github.com/cheahjs/free-llm-api-resources (2026-10).
type Preset struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	BaseURL  string `json:"baseUrl"`
	Protocol string `json:"protocol"`
	Note     string `json:"note"`   // free-tier shape, shown under the chip
	KeyURL   string `json:"keyUrl"` // where to apply for the key
}

// Presets is the catalog the dashboard renders. NVIDIA NIM leads (the user's
// ask); AMD is absent on purpose — AMD ships $100 Developer Cloud GPU credits
// rather than a hosted free API, so there is no endpoint to point at.
var Presets = []Preset{
	{ID: "nvidia-nim", Name: "NVIDIA NIM", BaseURL: "https://integrate.api.nvidia.com/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Đăng ký là được tặng hạn mức miễn phí (khoảng 40 lần/phút), hàng trăm mô hình mã nguồn mở: tiền tố deepseek-ai/, qwen/, meta/, nvidia/",
		KeyURL:   "https://build.nvidia.com"},
	{ID: "gemini", Name: "Google AI Studio (Gemini)", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
		Protocol: store.ProtocolOpenAI,
		Note:     "Gói miễn phí, giới hạn tốc độ theo mô hình; gemini-2.5-flash đủ dùng hằng ngày",
		KeyURL:   "https://aistudio.google.com/apikey"},
	{ID: "github-models", Name: "GitHub Models", BaseURL: "https://models.github.ai/inference",
		Protocol: store.ProtocolOpenAI,
		Note:     "GitHub PAT chính là Key, miễn phí giới hạn theo mô hình; ID mô hình dạng openai/gpt-4.1-mini",
		KeyURL:   "https://github.com/settings/tokens"},
	{ID: "groq", Name: "Groq", BaseURL: "https://api.groq.com/openai/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Gói miễn phí tốc độ cực nhanh (LPU), các mô hình mở như llama-3.3-70b",
		KeyURL:   "https://console.groq.com/keys"},
	{ID: "cerebras", Name: "Cerebras", BaseURL: "https://api.cerebras.ai/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Gói miễn phí, tốc độ suy luận waive-scale, mô hình mở llama/qwen",
		KeyURL:   "https://cloud.cerebras.ai"},
	{ID: "mistral", Name: "Mistral", BaseURL: "https://api.mistral.ai/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Gói thử nghiệm miễn phí (1 req/s), mistral-large / codestral",
		KeyURL:   "https://console.mistral.ai/api-keys"},
	{ID: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Cổng tổng hợp, mô hình có hậu tố :free trong ID là miễn phí (ví dụ deepseek/deepseek-r1:free)",
		KeyURL:   "https://openrouter.ai/keys"},
	{ID: "huggingface", Name: "HuggingFace Router", BaseURL: "https://router.huggingface.co/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Tặng hạn mức suy luận hằng tháng, định tuyến tới mô hình mở do nhiều bên host",
		KeyURL:   "https://huggingface.co/settings/tokens"},
	{ID: "siliconflow", Name: "SiliconFlow", BaseURL: "https://api.siliconflow.cn/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Kết nối thẳng trong nước (Trung Quốc), Qwen bản nhỏ, GLM-4-9B miễn phí thường trực",
		KeyURL:   "https://cloud.siliconflow.cn/account/ak"},
	{ID: "modelscope", Name: "ModelScope", BaseURL: "https://api-inference.modelscope.cn/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Kết nối thẳng trong nước (Trung Quốc), 2.000 lượt suy luận miễn phí mỗi ngày, trọn bộ mô hình mở",
		KeyURL:   "https://modelscope.cn/my/myaccesstoken"},
	{ID: "intern-ai", Name: "Intern (Thư Sinh · Đoan Nghiễn)", BaseURL: "https://discovery-api.intern-ai.org.cn/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Nền tảng mở của Phòng thí nghiệm AI Thượng Hải, làn miễn phí kết nối thẳng trong nước (Trung Quốc): hơn chục mô hình mở đầu bảng như DeepSeek-V4, GLM-5.3, Kimi-K2.6, Intern-S2, hỗ trợ hình ảnh và gọi công cụ",
		KeyURL:   "https://discovery.intern-ai.org.cn"},
	{ID: "zhipu", Name: "BigModel (Zhipu)", BaseURL: "https://open.bigmodel.cn/api/paas/v4",
		Protocol: store.ProtocolOpenAI,
		Note:     "Kết nối thẳng trong nước (Trung Quốc), dòng Flash như GLM-4.5-Flash miễn phí",
		KeyURL:   "https://open.bigmodel.cn/usercenter/apikeys"},
	{ID: "longcat", Name: "LongCat (Meituan)", BaseURL: "https://api.longcat.chat/openai/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Đăng ký là dùng, dòng LongCat-Flash, ngữ cảnh 1M",
		KeyURL:   "https://longcat.chat"},
	{ID: "longcat-anthropic", Name: "LongCat (giao thức Anthropic)", BaseURL: "https://api.longcat.chat/anthropic",
		Protocol: store.ProtocolAnthropic,
		Note:     "Cùng một nhà, đi giao thức Anthropic /messages, cấp cho các client kiểu Claude Code được",
		KeyURL:   "https://longcat.chat"},
	{ID: "anthropic", Name: "Anthropic chính thức", BaseURL: "https://api.anthropic.com/v1",
		Protocol: store.ProtocolAnthropic,
		Note:     "Trả phí (minh hoạ cách nối giao thức Anthropic), claude-opus-4-6 v.v.",
		KeyURL:   "https://console.anthropic.com/settings/keys"},
	{ID: "opencode-go", Name: "OpenCode Go", BaseURL: "https://opencode.ai/inference/go/v1",
		Protocol: store.ProtocolOpenAI,
		Note:     "Gói trả phí chính thức của OpenCode (thuê bao tháng, lấy Key trong bảng điều khiển OpenCode); cổng sẽ tự bổ sung header phiên mà nó yêu cầu, các client như Claude Code / Codex kết nối thẳng được",
		KeyURL:   "https://opencode.ai/docs/go/"},
}

// recommendPatterns lists, per provider, the model-id patterns worth enabling
// by default — curated from hands-on testing (NVIDIA free lane, Oct 2026:
// DeepSeek V4.1-Flash / Nemotron 3 Super / Nemotron 3 Ultra answer correctly
// at usable speeds; Kimi K3 and GLM-5.3 answer after 3-4 minute queues and are
// left out) and the provider's own free tier (Zhipu: the 4.x flash family).
var recommendPatterns = map[string][]string{
	"nvidia-nim": {
		"deepseek-ai/deepseek-v4.1-flash",
		"nvidia/nemotron-3-super",
		"nvidia/nemotron-3-ultra",
	},
	"integrate.api.nvidia.com": {
		"deepseek-ai/deepseek-v4.1-flash",
		"nvidia/nemotron-3-super",
		"nvidia/nemotron-3-ultra",
	},
	"zhipu": {
		"glm-4-flash", "glm-4.5-flash", "glm-4v-flash", "glm-4.1v-thinking-flash",
	},
	"open.bigmodel.cn": {
		"glm-4-flash", "glm-4.5-flash", "glm-4v-flash", "glm-4.1v-thinking-flash",
	},
	// SiliconFlow's free tier (verified Oct 2026 against a zero-balance key):
	// the Qwen small family answers for free; GLM-4-9B-0414, Hunyuan, Ling and
	// everything Pro/ are paid.
	"siliconflow": {
		"Qwen/Qwen2.5-7B-Instruct", "Qwen/Qwen3-8B", "Qwen/Qwen3-14B", "Qwen/Qwen3-32B",
	},
	"api.siliconflow.cn": {
		"Qwen/Qwen2.5-7B-Instruct", "Qwen/Qwen3-8B", "Qwen/Qwen3-14B", "Qwen/Qwen3-32B",
	},
	// ModelScope free inference (verified Oct 2026): the flagship tier answers
	// fast — DeepSeek-V4-Pro 1.5s, GLM-5.2 1.2s, Qwen3.5-397B 3.5s. MiniMax-M3
	// / ERNIE / GLM-4.7-Flash / LongCat-Lite returned empty deployments.
	"modelscope": {
		"deepseek-ai/DeepSeek-V4-Pro", "deepseek-ai/DeepSeek-V4.1-Flash",
		"ZhipuAI/GLM-5.2", "Qwen/Qwen3.5-397B", "Qwen/Qwen3.5-122B",
		"Qwen/Qwen3.8-Flash-Next", "stepfun-ai/Step-3.7-Flash",
	},
	"api-inference.modelscope.cn": {
		"deepseek-ai/DeepSeek-V4-Pro", "deepseek-ai/DeepSeek-V4.1-Flash",
		"ZhipuAI/GLM-5.2", "Qwen/Qwen3.5-397B", "Qwen/Qwen3.5-122B",
		"Qwen/Qwen3.8-Flash-Next", "stepfun-ai/Step-3.7-Flash",
	},
	// Intern dispatch API (verified Oct 2026 against a live key): every model
	// answers in under 7s first-token — deepseek-v4-pro 0.9s, intern-s2 1.3s,
	// glm-5.3 1.9s, qwen3.8-27b 0.5s, deepseek-v4-flash-vision 0.4s.
	// kimi-k2.6 is the slowest (6.6s) but worth the default tick; the Atria
	// mystery tier and Agents-A1 stay opt-in.
	"intern-ai": {
		"deepseek-v4-pro", "deepseek-v4-flash", "glm-5.3", "intern-s2",
		"kimi-k2.6", "qwen3.8-27b", "minimax-m3",
	},
	"discovery-api.intern-ai.org.cn": {
		"deepseek-v4-pro", "deepseek-v4-flash", "glm-5.3", "intern-s2",
		"kimi-k2.6", "qwen3.8-27b", "minimax-m3",
	},
}

// junkRe filters non-chat entries (embedders, safety nets, parsers, …) that
// clutter big catalogs like NVIDIA NIM's.
var junkRe = regexp.MustCompile(`(?i)embed|rerank|retriev|guard|reward|parse|whisper|image|diffusion|ocr|safety|topic-control|moderation|tts|speech|transcri|clip`)

// smallRe filters sub-33B dense models and small MoE (太落后太低端的不要).
// Anchors keep "120b-a12b" and "a12b" from matching the 12b alternative.
var smallRe = regexp.MustCompile(`(?i)(^|[^a-z0-9])(1|2|3|4|6\.7|7|8|9|11|12|14|20|22|24|30|31|32)b([^a-z0-9]|$)`)

// oldRe filters 2023-2024 vintage families.
var oldRe = regexp.MustCompile(`(?i)llama-?2|codellama|chatqa|mixtral|codestral|mistral-7b|mistral-nemo|minitron|recurrentgemma|deepseek-coder|llama-3\.[12]|nemotron-4-340b|gemma-2\b`)

// RecommendedModels picks the model ids a provider should enable by default:
// a curated list for known providers (matched by preset id or base-URL host,
// with word-boundary matching so glm-4-flash never drags in paid glm-4-flashx),
// falling back to a junk/small/vintage heuristic for unknown catalogs. The
// result never contains ids outside the fetched list.
func RecommendedModels(presetID, baseURL string, models []string) []string {
	pats := recommendPatterns[strings.ToLower(strings.TrimSpace(presetID))]
	if pats == nil {
		if host := hostOf(baseURL); host != "" {
			pats = recommendPatterns[strings.ToLower(host)]
		}
	}
	if pats != nil {
		var out []string
		for _, m := range models {
			for _, pat := range pats {
				if matchPattern(m, pat) {
					out = append(out, m)
					break
				}
			}
		}
		if len(out) > 0 {
			return out
		}
		// curated list matched nothing (catalog rotated) → heuristic below
	}
	out := []string{}
	for _, m := range models {
		if junkRe.MatchString(m) || smallRe.MatchString(m) || oldRe.MatchString(m) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// matchPattern reports whether id equals pat or extends it on a word boundary
// ("glm-4-flash" matches "glm-4-flash-250414" but not "glm-4-flashx").
func matchPattern(id, pat string) bool {
	id, pat = strings.ToLower(strings.TrimSpace(id)), strings.ToLower(strings.TrimSpace(pat))
	if id == pat {
		return true
	}
	if !strings.HasPrefix(id, pat) || len(id) == len(pat) {
		return false
	}
	switch id[len(pat)] {
	case '-', '_', '.', ':', '@', '/':
		return true
	}
	return false
}

func hostOf(baseURL string) string {
	u := NormalizeBaseURL(baseURL)
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.Index(u, "/"); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(u)
}
