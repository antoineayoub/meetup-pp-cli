package cli

import "testing"

func TestRequireFinalAbsoluteHTTPSURL(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "valid", value: "https://www.lewagon.com/events/example", want: true},
		{name: "relative", value: "/events/example", want: false},
		{name: "http", value: "http://www.lewagon.com/events/example", want: false},
		{name: "placeholder", value: "https://www.lewagon.com/events/<slug-de-l-evenement>", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requireFinalAbsoluteHTTPSURL(tc.value)
			if (err == nil) != tc.want {
				t.Fatalf("requireFinalAbsoluteHTTPSURL(%q) error = %v, want valid=%v", tc.value, err, tc.want)
			}
		})
	}
}
