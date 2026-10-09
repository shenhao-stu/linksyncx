package claude

import (
	"golang.org/x/mod/semver"
	"sync/atomic"
)

var sdkVersionResolver atomic.Pointer[cliVersionResolverFunc]

func IsSupportedSDKVersion(version string) bool {
	v := "v" + version
	return len(version) <= 64 && semver.IsValid(v) && semver.Canonical(v) == v &&
		semver.Prerelease(v) == "" && semver.Compare(v, "v"+SDKTSVersion) >= 0
}

func SetSDKVersionResolver(resolver func() string) {
	if resolver == nil {
		sdkVersionResolver.Store(nil)
		return
	}
	r := cliVersionResolverFunc(resolver)
	sdkVersionResolver.Store(&r)
}

func EffectiveSDKVersion() string {
	if r := sdkVersionResolver.Load(); r != nil {
		if v := (*r)(); IsSupportedSDKVersion(v) {
			return v
		}
	}
	return SDKTSVersion
}
