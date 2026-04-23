// Package gateway implements the API Gateway head of Hydra.
//
// The gateway is the mouth of the organism. It receives all inbound HTTP
// traffic, normalizes requests into Envelopes, routes them to the correct
// handler, and returns normalized responses.
package gateway
