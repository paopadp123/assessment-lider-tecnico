namespace Issuing.Api.Legacy;

// Contrato del emisor legado (simulator/README.md). Verboso y con campos redundantes que deben coincidir;
// no sale de esta carpeta: el resto del servicio solo conoce el dominio.

public sealed record LegacyIssueRequest(LegacyHeader Header, LegacyPolicyRequest PolicyRequest);

public sealed record LegacyHeader(string TransactionId, string Channel, string RequestDate);

public sealed record LegacyPolicyRequest(
    string ExternalRef,
    string ExternalReference,
    LegacyProduct Product,
    LegacyVehicle Vehicle,
    LegacyHolder Holder,
    LegacyPremium Premium);

public sealed record LegacyProduct(string Code, string ProductCode);

public sealed record LegacyVehicle(string Plate, string PlateNumber, string ModelYear, string CityCode);

public sealed record LegacyHolder(LegacyDocument Document, string DocumentNumber);

public sealed record LegacyDocument(string Type, string Number);

public sealed record LegacyPremium(string Amount, decimal Value, string Currency);

/// <summary>Respuesta de POST /issue. Siempre llega con 200: el error real está en <see cref="Result"/> y <see cref="Code"/>.</summary>
public sealed record LegacyIssueResponse(string? Result, string? Code, string? Message, LegacyIssueData? Data);

public sealed record LegacyIssueData(LegacyPolicy? Policy);

public sealed record LegacyPolicy(string? PolicyNumber, string? ExternalRef);

/// <summary>Respuesta de GET /policies?externalRef=. Lista vacía si la póliza no existe.</summary>
public sealed record LegacyLookupResponse(string? Result, string? Code, IReadOnlyList<LegacyPolicy>? Policies);
