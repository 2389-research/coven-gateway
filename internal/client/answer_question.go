// ABOUTME: ClientService gRPC handler for answering user questions
// ABOUTME: Implements AnswerQuestion RPC for responding to ask_user tool requests

package client

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// QuestionAnswerer defines the interface for delivering user question answers
type QuestionAnswerer interface {
	DeliverAnswer(agentID, questionID string, answer *pb.AnswerQuestionRequest) error
}

// SetQuestionAnswerer sets the question answerer for user question operations
func (s *ClientService) SetQuestionAnswerer(answerer QuestionAnswerer) {
	s.answerer = answerer
}

// AnswerQuestion responds to a user question from the ask_user tool
func (s *ClientService) AnswerQuestion(ctx context.Context, req *pb.AnswerQuestionRequest) (*pb.AnswerQuestionResponse, error) {
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id required")
	}
	if req.QuestionId == "" {
		return nil, status.Error(codes.InvalidArgument, "question_id required")
	}

	if s.answerer == nil {
		return &pb.AnswerQuestionResponse{
			Success: false,
			Error:   strPtr("question answerer not configured"),
		}, nil
	}

	err := s.answerer.DeliverAnswer(req.AgentId, req.QuestionId, req)
	if err != nil {
		return &pb.AnswerQuestionResponse{
			Success: false,
			Error:   strPtr(err.Error()),
		}, nil
	}

	return &pb.AnswerQuestionResponse{
		Success: true,
	}, nil
}
