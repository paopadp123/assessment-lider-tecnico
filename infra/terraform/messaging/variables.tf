variable "environment" {
  description = "Nombre del entorno (staging, production). Va en el nombre de cada recurso."
  type        = string

  validation {
    condition     = contains(["staging", "production"], var.environment)
    error_message = "environment debe ser staging o production."
  }
}

variable "orchestrator_task_role_name" {
  description = "Rol IAM de la tarea ECS del orquestador: es el único que publica y consume la cola."
  type        = string
}

variable "alarm_topic_arn" {
  description = "Tópico SNS de guardia (PagerDuty/Opsgenie) para las alarmas que despiertan a alguien."
  type        = string

  validation {
    condition     = startswith(var.alarm_topic_arn, "arn:aws:sns:")
    error_message = "alarm_topic_arn debe ser el ARN de un tópico SNS."
  }
}

variable "ticket_topic_arn" {
  description = "Tópico SNS que abre un ticket en horario laboral, sin despertar a nadie."
  type        = string

  validation {
    condition     = startswith(var.ticket_topic_arn, "arn:aws:sns:")
    error_message = "ticket_topic_arn debe ser el ARN de un tópico SNS."
  }
}

variable "issue_visibility_timeout_seconds" {
  description = "Debe superar un intento completo: hasta dos llamadas a la capa de emisión (consulta y emisión) de 10 s cada una, más el margen de procesamiento."
  type        = number
  default     = 60

  validation {
    condition     = var.issue_visibility_timeout_seconds >= 30 && var.issue_visibility_timeout_seconds <= 43200
    error_message = "El visibility timeout debe estar entre 30 s (un intento completo) y 43200 s (máximo de SQS)."
  }
}

variable "issue_max_receive_count" {
  description = "Recepciones antes de mover el comando a la DLQ. Igual a MaxAttempts del orquestador (4, issue_policy.go) para que SQS y la aplicación cuenten lo mismo. SQS cuenta cada recepción, así que con el breaker abierto el worker no devuelve el mensaje: lo reencola como uno nuevo con DelaySeconds y borra el original (ADR 0003)."
  type        = number
  default     = 4

  validation {
    condition     = var.issue_max_receive_count >= 1 && var.issue_max_receive_count <= 1000
    error_message = "maxReceiveCount debe estar entre 1 y 1000 (límites de SQS)."
  }
}

variable "oldest_message_alarm_seconds" {
  description = "Segundos que una orden pagada puede estar sin póliza (o un comando sin consumirse) antes de llamar a guardia."
  type        = number
  default     = 600

  validation {
    condition     = var.oldest_message_alarm_seconds > 0
    error_message = "El umbral de la alarma debe ser mayor que cero."
  }
}
