package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

// ccVersionInBillingRe matches the semver part of cc_version (X.Y.Z).
var ccVersionInBillingRe = regexp.MustCompile(`cc_version=\d+\.\d+\.\d+`)

var ccVersionWithFingerprintInBillingRe = regexp.MustCompile(`cc_version=\d+\.\d+\.\d+\.[0-9a-fA-F]{3}\b`)

// effectiveBillingUserAgent 选择写进 x-anthropic-billing-header 的 User-Agent。
// OAuth mimicry 强制使用调用方传入的 mimicUserAgent（与出站 User-Agent 头同源、
// 同一次请求内取一次复用，保证 cc_version 与出站头版本严格一致），
// 其余情况使用账号指纹 UA。
func effectiveBillingUserAgent(mimicUserAgent, tokenType string, mimicClaudeCode bool, fingerprint *Fingerprint) string {
	if tokenType == "oauth" && mimicClaudeCode {
		return mimicUserAgent
	}
	if fingerprint == nil {
		return ""
	}
	return fingerprint.UserAgent
}

// syncBillingHeaderVersion rewrites cc_version in x-anthropic-billing-header
// system text blocks to match the version extracted from userAgent.
// Recompute any recognized fingerprint suffix because its input includes the version.
// Only touches system array blocks whose text starts with "x-anthropic-billing-header".
func syncBillingHeaderVersion(body []byte, userAgent string) []byte {
	view := newJSONBodyView(body, nil)
	syncBillingHeaderVersionView(view, userAgent)
	return view.data
}

// syncBillingHeaderVersionView 是 syncBillingHeaderVersion 作用于 jsonBodyView 的版本。
func syncBillingHeaderVersionView(view *jsonBodyView, userAgent string) {
	version := ExtractCLIVersion(userAgent)
	if version == "" {
		return
	}

	systemResult := view.get("system")
	if !systemResult.Exists() || !systemResult.IsArray() {
		return
	}

	replacement := "cc_version=" + version
	idx := 0
	systemResult.ForEach(func(_, item gjson.Result) bool {
		text := item.Get("text")
		if text.Exists() && text.Type == gjson.String &&
			strings.HasPrefix(text.String(), "x-anthropic-billing-header") {
			fingerprintedReplacement := replacement + "." + computeClaudeCodeFingerprintView(view, version)
			newText := ccVersionWithFingerprintInBillingRe.ReplaceAllString(text.String(), fingerprintedReplacement)
			newText = ccVersionInBillingRe.ReplaceAllString(newText, replacement)

			if newText != text.String() {
				_ = view.setString(fmt.Sprintf("system.%d.text", idx), newText)
			}
		}
		idx++
		return true
	})
}
