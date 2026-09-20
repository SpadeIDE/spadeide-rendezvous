package signal

import "testing"

func FuzzParseControlMessage(f *testing.F) {
	f.Add([]byte(`{"t":"hello","aid":"abc","proto":1}`))
	f.Add([]byte(`{"t":"hello","rids":["a","b"],"proto":2}`))
	f.Add([]byte(`{"t":"prove","pub":"x","sig":"y","mac":"z"}`))
	f.Add([]byte(`{"t":"lookup","aid":"abc","mode":"full"}`))
	f.Add([]byte(`{"t":"lookup","rid":"opq","mode":"punch"}`))
	f.Add([]byte(`{"t":"ping"}`))
	f.Add([]byte(`{`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		msg, err := ParseControlMessage(raw)
		if err != nil {
			return
		}
		if msg == nil {
			t.Fatal("nil msg without error")
		}
		_, _ = msg["t"]
		_ = parseRIDs(msg)
	})
}
