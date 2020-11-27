















package graphql

import (
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/node"
	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)


func New(stack *node.Node, backend ethapi.Backend, cors, vhosts []string) error {
	if backend == nil {
		panic("missing backend")
	}
	
	return newHandler(stack, backend, cors, vhosts)
}



func newHandler(stack *node.Node, backend ethapi.Backend, cors, vhosts []string) error {
	q := Resolver{backend}

	s, err := graphql.ParseSchema(schema, &q)
	if err != nil {
		return err
	}
	h := &relay.Handler{Schema: s}
	handler := node.NewHTTPHandlerStack(h, cors, vhosts)

	stack.RegisterHandler("GraphQL UI", "/graphql/ui", GraphiQL{})
	stack.RegisterHandler("GraphQL", "/graphql", handler)
	stack.RegisterHandler("GraphQL", "/graphql/", handler)

	return nil
}
