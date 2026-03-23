package server

import "testing"

func TestPathSuffixTreatsRootDocumentsAsMappedBase(t *testing.T) {
	cases := []struct {
		name string
		base string
		path string
		want string
	}{
		{name: "root slash routes to mapped base", base: "/ops", path: "/", want: ""},
		{name: "root index routes to mapped base", base: "/ops", path: "/index.html", want: ""},
		{name: "default document under base routes to mapped base", base: "/ops", path: "/ops/default.html", want: ""},
		{name: "home document under base routes to mapped base", base: "/ops", path: "/ops/home.htm", want: ""},
		{name: "suffix path stays suffix", base: "/ops", path: "/ops/assets/app.js", want: "/assets/app.js"},
		{name: "duplicate slashes default doc routes to mapped base", base: "/ops", path: "//ops//index.html", want: ""},
		{name: "dot segment default doc routes to mapped base", base: "/ops", path: "/ops/./index.html", want: ""},
		{name: "dot segment asset path keeps normalized suffix", base: "/ops", path: "/ops/assets/../assets/app.js", want: "/assets/app.js"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pathSuffix(tc.base, tc.path)
			if !ok {
				t.Fatalf("pathSuffix(%q, %q) unexpectedly failed", tc.base, tc.path)
			}
			if got != tc.want {
				t.Fatalf("pathSuffix(%q, %q)=%q, want %q", tc.base, tc.path, got, tc.want)
			}
		})
	}
}
