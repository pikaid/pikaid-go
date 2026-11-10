package pikaid

import "testing"

const benchID = "0s44we8002loz3po6mcf6vsn23"

func BenchmarkParse(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(benchID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkString(b *testing.B) {
	id, err := Parse(benchID)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if s := id.String(); len(s) != StringLength {
			b.Fatal("unexpected length")
		}
	}
}

func BenchmarkGenerate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Generate(); err != nil {
			b.Fatal(err)
		}
	}
}
