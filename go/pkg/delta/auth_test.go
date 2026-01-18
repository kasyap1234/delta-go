package delta

import "testing"

func TestGenerateSignature_MatchesDocsExample(t *testing.T) {
	secret := "7b6f39dcf660ec1c7c664f612c60410a2bd0c258416b498bf0311f94228f"
	method := "GET"
	timestamp := "1542110948"
	path := "/v2/orders"
	queryString := "product_id=1&state=open"
	body := ""

	got := GenerateSignature(secret, method, timestamp, path, queryString, body)
	want := "4e38dda3e6477092f360ba70399266d8145630b22bcc34c0ec7f804d5746877a"
	if got != want {
		t.Fatalf("signature mismatch: got=%s want=%s", got, want)
	}
}
