package runtime

func AddInt64(a, b IntValue) IntValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}
	return IntValueOf(a.v + b.v)
}
