















package rpc

import "fmt"

var (
	_ Error = new(methodNotFoundError)
	_ Error = new(subscriptionNotFoundError)
	_ Error = new(parseError)
	_ Error = new(invalidRequestError)
	_ Error = new(invalidMessageError)
	_ Error = new(invalidParamsError)
)

const defaultErrorCode = -32000

type methodNotFoundError struct{ method string }

func (e *methodNotFoundError) ErrorCode() int { return -32601 }

func (e *methodNotFoundError) Error() string {
	return fmt.Sprintf("the method %s does not exist/is not available", e.method)
}

type subscriptionNotFoundError struct{ namespace, subscription string }

func (e *subscriptionNotFoundError) ErrorCode() int { return -32601 }

func (e *subscriptionNotFoundError) Error() string {
	return fmt.Sprintf("no %q subscription in %s namespace", e.subscription, e.namespace)
}


type parseError struct{ message string }

func (e *parseError) ErrorCode() int { return -32700 }

func (e *parseError) Error() string { return e.message }


type invalidRequestError struct{ message string }

func (e *invalidRequestError) ErrorCode() int { return -32600 }

func (e *invalidRequestError) Error() string { return e.message }


type invalidMessageError struct{ message string }

func (e *invalidMessageError) ErrorCode() int { return -32700 }

func (e *invalidMessageError) Error() string { return e.message }


type invalidParamsError struct{ message string }

func (e *invalidParamsError) ErrorCode() int { return -32602 }

func (e *invalidParamsError) Error() string { return e.message }
