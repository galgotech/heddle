package interaction

// Match representa o retorno estruturado para pattern matching na stdlib
type Match[T ~string, V any] struct {
	Tag T
	Val V
}

// Seq representa um gerador de valores de tipo V sem canal de resposta (unidirecional)
type Seq[T ~string, V any] func(yield func(Match[T, V]) bool)

// ReqReply representa um gerador de requisições de tipo Req que espera uma resposta de tipo Resp (bidirecional)
type ReqReply[T ~string, Req any, Resp any] func(yield func(Req) Resp)

// SeqAccum representa um gerador acumulador de valores (unidirecional)
type SeqAccum[T ~string, V any] func(yield func(Match[T, V]) bool)
