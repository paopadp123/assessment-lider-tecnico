namespace Issuing.Api.Domain;

/// <summary>
/// Solicitud de emisión tal como llega del orquestador. Todo es anulable porque viene de afuera:
/// <see cref="IssuePolicyValidator"/> la convierte en un <see cref="IssuePolicyCommand"/> válido o en errores por campo.
/// </summary>
public sealed record IssuePolicyRequest(
    string? ExternalRef,
    string? QuoteId,
    decimal? Premium,
    string? Currency,
    string? Plate,
    string? DocumentId,
    int? VehicleYear,
    string? CityCode);

/// <summary>Solicitud de emisión ya validada: ningún campo falta ni tiene un formato inválido.</summary>
public sealed record IssuePolicyCommand(
    string ExternalRef,
    string QuoteId,
    decimal Premium,
    string Currency,
    string Plate,
    string DocumentId,
    int VehicleYear,
    string CityCode);
