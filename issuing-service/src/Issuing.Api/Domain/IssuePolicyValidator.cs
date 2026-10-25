using System.Text.RegularExpressions;

namespace Issuing.Api.Domain;

/// <summary>Resultado de validar una <see cref="IssuePolicyRequest"/>.</summary>
public abstract record ValidationOutcome
{
    private ValidationOutcome()
    {
    }

    public sealed record Valid(IssuePolicyCommand Command) : ValidationOutcome;

    public sealed record Invalid(IReadOnlyDictionary<string, string[]> Errors) : ValidationOutcome;
}

/// <summary>
/// Reglas de la solicitud de emisión. Son las mismas que valida el orquestador al crear la orden
/// (orchestrator/internal/order): la ACL no confía en que su único cliente las haya aplicado.
/// </summary>
public static partial class IssuePolicyValidator
{
    public const int MinVehicleYear = 1980;

    private const int MaxExternalRefLength = 64;

    public static ValidationOutcome Validate(IssuePolicyRequest request, int currentYear)
    {
        ArgumentNullException.ThrowIfNull(request);
        Dictionary<string, string[]> errors = [];

        string externalRef = Required(request.ExternalRef, "externalRef", errors);
        if (externalRef.Length > MaxExternalRefLength)
        {
            errors["externalRef"] = [$"must be at most {MaxExternalRefLength} characters"];
        }

        string quoteId = Required(request.QuoteId, "quoteId", errors);
        string currency = Matching(request.Currency, CurrencyFormat(), "currency", "must be an ISO 4217 code (e.g. COP)", errors);
        string plate = Matching(request.Plate, PlateFormat(), "plate", "must be 3 letters, 2 digits and a letter or digit", errors);
        string documentId = Matching(request.DocumentId, DocumentFormat(), "documentId", "must have 5 to 10 digits", errors);
        string cityCode = Matching(request.CityCode, CityCodeFormat(), "cityCode", "must be a 5-digit DANE code", errors);

        decimal premium = request.Premium ?? 0m;
        if (premium <= 0m)
        {
            errors["premium"] = ["must be greater than 0"];
        }
        else if (premium != decimal.Round(premium, 2))
        {
            // El legado repite la prima como texto con 2 decimales y como número: deben coincidir.
            errors["premium"] = ["must have at most 2 decimals"];
        }

        int vehicleYear = request.VehicleYear ?? 0;
        int maxYear = currentYear + 1;
        if (vehicleYear < MinVehicleYear || vehicleYear > maxYear)
        {
            errors["vehicleYear"] = [$"must be between {MinVehicleYear} and {maxYear}"];
        }

        if (errors.Count > 0)
        {
            return new ValidationOutcome.Invalid(errors);
        }

        return new ValidationOutcome.Valid(
            new IssuePolicyCommand(externalRef, quoteId, premium, currency, plate, documentId, vehicleYear, cityCode));
    }

    private static string Required(string? value, string field, Dictionary<string, string[]> errors)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            errors[field] = ["is required"];
            return string.Empty;
        }

        return value;
    }

    private static string Matching(string? value, Regex format, string field, string message, Dictionary<string, string[]> errors)
    {
        string text = Required(value, field, errors);
        if (text.Length > 0 && !format.IsMatch(text))
        {
            errors[field] = [message];
        }

        return text;
    }

    // \z y no $: en .NET, $ también acepta un salto de línea final ("ABC123" + "\n" pasaría).
    [GeneratedRegex(@"^[A-Z]{3}\z")]
    private static partial Regex CurrencyFormat();

    [GeneratedRegex(@"^[A-Z]{3}[0-9]{2}[0-9A-Z]\z")]
    private static partial Regex PlateFormat();

    [GeneratedRegex(@"^[0-9]{5,10}\z")]
    private static partial Regex DocumentFormat();

    [GeneratedRegex(@"^[0-9]{5}\z")]
    private static partial Regex CityCodeFormat();
}
