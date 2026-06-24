package txsizes

import (
	"testing"

	"github.com/modelos/modelos/node/wire"
)

func TestEstimateSerializeSizeTaproot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		inputCount           int
		outputScriptLengths  []int
		addChangeOutput      bool
		expectedSizeEstimate int
	}{
		{
			name:                 "1 input, no outputs, no change",
			inputCount:           1,
			outputScriptLengths:  []int{},
			addChangeOutput:      false,
			expectedSizeEstimate: 8 + 1 + 1 + 41, // version+locktime + inputs_varint + outputs_varint + input_size
		},
		{
			name:                 "1 input, 1 P2TR output, no change",
			inputCount:           1,
			outputScriptLengths:  []int{P2TRPkScriptSize},
			addChangeOutput:      false,
			expectedSizeEstimate: 8 + 1 + 1 + 41 + 43, // + P2TR output
		},
		{
			name:                 "1 input, 1 P2TR output, with P2TR change",
			inputCount:           1,
			outputScriptLengths:  []int{P2TRPkScriptSize},
			addChangeOutput:      true,
			expectedSizeEstimate: 8 + 1 + 1 + 41 + 43 + 43, // + another P2TR output for change
		},
		{
			name:                 "2 inputs, 1 P2TR output, no change",
			inputCount:           2,
			outputScriptLengths:  []int{P2TRPkScriptSize},
			addChangeOutput:      false,
			expectedSizeEstimate: 8 + 1 + 1 + (41 * 2) + 43, // 2 inputs
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outputs := make([]*wire.TxOut, 0, len(test.outputScriptLengths))
			for _, scriptLen := range test.outputScriptLengths {
				script := make([]byte, scriptLen)
				outputs = append(outputs, wire.NewTxOut(0, script))
			}

			estimated := EstimateSerializeSize(test.inputCount, outputs, test.addChangeOutput)
			if estimated != test.expectedSizeEstimate {
				t.Errorf("Expected estimated size to be %d, got %d",
					test.expectedSizeEstimate, estimated)
			}
		})
	}
}

func TestEstimateVirtualSizeTaproot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		numP2TRIns       int
		outputs          []*wire.TxOut
		changeScriptSize int
		expectedVSize    int
	}{
		{
			name:             "1 P2TR input, 1 P2TR output, no change",
			numP2TRIns:       1,
			outputs:          []*wire.TxOut{wire.NewTxOut(100000, make([]byte, P2TRPkScriptSize))},
			changeScriptSize: 0,
			// Base: 8 + 1 + 1 + 41 + 43 = 94
			// Witness: (2 + 1 + 67 + 3) / 4 = 18.25 -> 18
			// Total: 94 + 18 = 112
			expectedVSize: 112,
		},
		{
			name:             "1 P2TR input, 1 P2TR output, with P2TR change",
			numP2TRIns:       1,
			outputs:          []*wire.TxOut{wire.NewTxOut(100000, make([]byte, P2TRPkScriptSize))},
			changeScriptSize: P2TRPkScriptSize,
			// Base: 8 + 1 + 1 + 41 + 43 + 43 = 137 (with change output)
			// Witness: (2 + 1 + 67 + 3) / 4 = 18.25 -> 18
			// Total: 137 + 18 = 155
			expectedVSize: 155,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Use legacy signature but only pass P2TR inputs
			estimated := EstimateVirtualSize(0, test.numP2TRIns, 0, 0, test.outputs, test.changeScriptSize)
			if estimated != test.expectedVSize {
				t.Errorf("Expected virtual size to be %d, got %d",
					test.expectedVSize, estimated)
			}
		})
	}
}
