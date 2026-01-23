package main

func SimpleHandler(args string, context string) (string, error) {
	return "", nil
}

func StreamHandler(args string, context string, ch chan<- string) error {
	return nil
}
