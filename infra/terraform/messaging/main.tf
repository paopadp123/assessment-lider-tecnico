# Cola de comandos de emisión de pólizas: el componente donde se pierde dinero si falla.
# Reemplaza en AWS a la cola issue_policy de RabbitMQ (ADR 0002). La outbox en Aurora
# no cambia: el despachador publica aquí en vez de en RabbitMQ.

locals {
  name = "policy-${var.environment}"
  tags = {
    Service     = "orchestrator"
    Environment = var.environment
    DataClass   = "personal" # los comandos llevan el id de la orden, no placa ni documento
  }
}

# Los mensajes que agotan sus intentos quedan aquí para que operaciones decida.
# 14 días (el máximo de SQS) para que nadie pierda un comando por un fin de semana largo.
resource "aws_sqs_queue" "issue_policy_dlq" {
  name                      = "${local.name}-issue-policy-dlq"
  message_retention_seconds = 1209600
  sqs_managed_sse_enabled   = true
  tags                      = local.tags
}

resource "aws_sqs_queue" "issue_policy" {
  name                       = "${local.name}-issue-policy"
  visibility_timeout_seconds = var.issue_visibility_timeout_seconds
  message_retention_seconds  = 345600 # 4 días: el reconciliador recupera antes lo que quede atascado
  receive_wait_time_seconds  = 20     # long polling: menos llamadas vacías y menos costo
  sqs_managed_sse_enabled    = true

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.issue_policy_dlq.arn
    maxReceiveCount     = var.issue_max_receive_count
  })

  tags = local.tags
}

# Solo la cola principal puede mandar mensajes a esta DLQ.
resource "aws_sqs_queue_redrive_allow_policy" "issue_policy_dlq" {
  queue_url = aws_sqs_queue.issue_policy_dlq.id

  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue"
    sourceQueueArns   = [aws_sqs_queue.issue_policy.arn]
  })
}

# Mínimo privilegio: el orquestador publica, consume y extiende la visibilidad de un comando
# lento. Con el breaker abierto aplaza reencolando con DelaySeconds (SendMessage) y borrando
# el original, porque ChangeMessageVisibility no reinicia el contador de recepciones y el
# comando acabaría en la DLQ. No puede borrar ni purgar colas.
data "aws_iam_policy_document" "orchestrator_issue_policy" {
  statement {
    sid = "PublishAndConsumeIssueCommands"
    actions = [
      "sqs:SendMessage",
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:ChangeMessageVisibility",
      "sqs:GetQueueAttributes",
    ]
    resources = [aws_sqs_queue.issue_policy.arn]
  }

  statement {
    sid       = "InspectDeadLetters"
    actions   = ["sqs:ReceiveMessage", "sqs:GetQueueAttributes"]
    resources = [aws_sqs_queue.issue_policy_dlq.arn]
  }
}

resource "aws_iam_role_policy" "orchestrator_issue_policy" {
  name   = "${local.name}-issue-policy-queue"
  role   = var.orchestrator_task_role_name
  policy = data.aws_iam_policy_document.orchestrator_issue_policy.json
}

# Alarma que despierta a alguien: una orden pagada lleva más de 10 minutos sin póliza.
# Mide el síntoma que siente el cliente, no la causa. Sale de la base y no de SQS, porque cada
# aplazamiento reencola el comando como un mensaje nuevo y reinicia su edad en la cola.
# Diseño, aún no implementado en el orquestador: el reconciliador la publicaría cada minuto,
# para las órdenes en PAID o ISSUING, como el máximo de now() - occurred_at del evento de
# order_events que las llevó a PAID.
resource "aws_cloudwatch_metric_alarm" "oldest_unissued_paid_order" {
  alarm_name          = "${local.name}-oldest-unissued-paid-order"
  alarm_description   = "Hay órdenes pagadas sin póliza hace más de ${var.oldest_message_alarm_seconds} s. Runbook: docs/architecture.md#runbook-emision"
  namespace           = "PolicyPlatform/Orchestrator"
  metric_name         = "OldestUnissuedPaidOrderSeconds"
  dimensions          = { Environment = var.environment }
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 5
  datapoints_to_alarm = 5 # cinco minutos seguidos: un pico aislado no despierta a nadie
  threshold           = var.oldest_message_alarm_seconds
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "breaching" # si el reconciliador deja de publicar, también es un problema
  alarm_actions       = [var.alarm_topic_arn]
  ok_actions          = [var.alarm_topic_arn]
  tags                = local.tags
}

# Alarma que despierta a alguien: nadie consume la cola (workers caídos o sin permisos).
# Con el breaker abierto no se dispara, porque los comandos se reencolan y su edad se reinicia.
resource "aws_cloudwatch_metric_alarm" "issue_policy_oldest_message" {
  alarm_name          = "${local.name}-issue-policy-oldest-message"
  alarm_description   = "Un comando de emisión lleva más de ${var.oldest_message_alarm_seconds} s sin consumirse: revisar las tareas del worker. Runbook: docs/architecture.md#runbook-emision"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateAgeOfOldestMessage"
  dimensions          = { QueueName = aws_sqs_queue.issue_policy.name }
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 5
  datapoints_to_alarm = 5
  threshold           = var.oldest_message_alarm_seconds
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = [var.alarm_topic_arn]
  ok_actions          = [var.alarm_topic_arn]
  tags                = local.tags
}

# Alarma que abre un ticket: un comando agotó sus intentos. La orden queda en ISSUING (un
# timeout no prueba que la póliza no exista) y la reconciliación consulta y decide (ADR 0003);
# operaciones lo sigue desde la consola. No es una emergencia.
resource "aws_cloudwatch_metric_alarm" "issue_policy_dlq_not_empty" {
  alarm_name          = "${local.name}-issue-policy-dlq-not-empty"
  alarm_description   = "Hay comandos de emisión en la DLQ. Revisar las órdenes en la consola de operaciones."
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  dimensions          = { QueueName = aws_sqs_queue.issue_policy_dlq.name }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = [var.ticket_topic_arn]
  tags                = local.tags
}
