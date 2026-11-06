package pikaid_test

import (
	"encoding/hex"
	"fmt"
	"log"

	pikaid "github.com/pikaid/pikaid-go"
)

func Example() {
	id, err := pikaid.Parse("0s44we8002loz3po6mcf6vsn23")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(id.String())
	fmt.Println(id.Timestamp())
	fmt.Println(id.RandomnessHex())

	// Output:
	// 0s44we8002loz3po6mcf6vsn23
	// 1700000000
	// 00112233445566778899aabb
}

func ExampleGenerateWithTimestamp() {
	id, err := pikaid.GenerateWithTimestamp(1_700_000_000)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(id.Timestamp())
	fmt.Println(len(id.String()))
	fmt.Println(pikaid.IsValid(id.String()))

	// Output:
	// 1700000000
	// 26
	// true
}

func ExampleFromBinary() {
	raw, err := hex.DecodeString("006553f10000112233445566778899aabb")
	if err != nil {
		log.Fatal(err)
	}

	id, err := pikaid.FromBinary(raw)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(id)
	fmt.Printf("%x\n", id.Bytes())

	// Output:
	// 0s44we8002loz3po6mcf6vsn23
	// 006553f10000112233445566778899aabb
}

func ExampleID_Compare() {
	earlier, err := pikaid.Parse("00000010000000000000000001")
	if err != nil {
		log.Fatal(err)
	}
	later, err := pikaid.Parse("00000020000000000000000001")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(earlier.Compare(later))
	fmt.Println(later.Compare(earlier))
	fmt.Println(earlier.Compare(earlier))

	// Output:
	// -1
	// 1
	// 0
}
