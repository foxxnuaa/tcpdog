package helper

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mehrdadrad/tcpdog/config"
)

var cfg = config.Config{
	Output: map[string]config.OutputConfig{
		"myoutput": {
			Type: "grpc2",
		},
	},
	Fields: map[string][]config.Field{
		"myfields": {
			{Name: "Task"},
			{Name: "Fake1"},
			{Name: "Fake2"},
		},
	},
}

func TestPBStructUnmarshal(t *testing.T) {
	spb := NewStructPB(cfg.Fields["myfields"])
	spb.hostname = "fakehost"
	buf := bytes.NewBufferString(`{"Task":"curl","Fake1":1,"Fake2":2,"Timestamp":1609720926}`)
	r := spb.Unmarshal(buf)

	assert.Equal(t, "curl", r.Fields["Task"].GetStringValue())
	assert.Equal(t, 1.0, r.Fields["Fake1"].GetNumberValue())
	assert.Equal(t, 2.0, r.Fields["Fake2"].GetNumberValue())
	assert.Equal(t, "fakehost", r.Fields["Hostname"].GetStringValue())

}

func BenchmarkPBStructUnmarshal(b *testing.B) {
	spb := NewStructPB(cfg.Fields["myfields"])

	for i := 0; i < b.N; i++ {
		buf := bytes.NewBufferString(`{"Task":"curl","Fake1":1,"Fake2":2,"Timestamp":1609720926}`)
		spb.Unmarshal(buf)
	}
}