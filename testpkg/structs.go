package test

type Pony struct {
	Name *string
	Age  *int64
}

type Narwhal struct {
	Name *string
	Age  *int64
	Horn *Horn
}

type Horn struct {
	Length *float64
	Color  *string
}
