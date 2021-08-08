package peer

func sizeOf(data [][]byte) int {
	var sum int
	for _, v := range data {
		sum += len(v)
	}

	return sum
}

func flatten(data [][]byte) []byte {
	var out []byte
	for _, chunk := range data {
		out = append(out, chunk...)
	}

	return out
}

