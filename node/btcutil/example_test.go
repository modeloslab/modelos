package btcutil_test

import (
	"fmt"
	"math"

	"github.com/modelos/modelos/node/btcutil"
)

func ExampleAmount() {

	a := btcutil.Amount(0)
	fmt.Println("Zero Grain:", a)

	a = btcutil.Amount(1e8)
	fmt.Println("100,000,000 Grains:", a)

	a = btcutil.Amount(1e5)
	fmt.Println("100,000 Grains:", a)
	// Output:
	// Zero Grain: 0 PRL
	// 100,000,000 Grains: 1 PRL
	// 100,000 Grains: 0.00100000 PRL
}

func ExampleNewAmount() {
	amountOne, err := btcutil.NewAmount(1)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountOne) //Output 1

	amountFraction, err := btcutil.NewAmount(0.01234567)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountFraction) //Output 2

	amountZero, err := btcutil.NewAmount(0)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountZero) //Output 3

	amountNaN, err := btcutil.NewAmount(math.NaN())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountNaN) //Output 4

	// Output: 1 PRL
	// 0.01234567 PRL
	// 0 PRL
	// invalid pearl amount
}

func ExampleAmount_unitConversions() {
	amount := btcutil.Amount(44433322211100)

	fmt.Println("Grain to kPRL:", amount.Format(btcutil.AmountKiloPRL))
	fmt.Println("Grain to PRL:", amount)
	fmt.Println("Grain to MilliPRL:", amount.Format(btcutil.AmountMilliPRL))
	fmt.Println("Grain to MicroPRL:", amount.Format(btcutil.AmountMicroPRL))
	fmt.Println("Grain to Grain:", amount.Format(btcutil.AmountGrain))

	// Output:
	// Grain to kPRL: 444.333222111 kPRL
	// Grain to PRL: 444333.22211100 PRL
	// Grain to MilliPRL: 444333222.111 mPRL
	// Grain to MicroPRL: 444333222111 μPRL
	// Grain to Grain: 44433322211100 Grain
}
