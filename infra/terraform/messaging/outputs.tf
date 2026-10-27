output "issue_policy_queue_url" {
  description = "Va a la configuración del orquestador (SSM Parameter Store), no a un secreto."
  value       = aws_sqs_queue.issue_policy.id
}

output "issue_policy_queue_arn" {
  value = aws_sqs_queue.issue_policy.arn
}

output "issue_policy_dlq_arn" {
  value = aws_sqs_queue.issue_policy_dlq.arn
}
