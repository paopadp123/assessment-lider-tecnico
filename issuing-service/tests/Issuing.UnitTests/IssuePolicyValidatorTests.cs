using Issuing.Api.Domain;

namespace Issuing.UnitTests;

public sealed class IssuePolicyValidatorTests
{
    private const int CurrentYear = 2026;

    public static readonly IssuePolicyRequest ValidRequest = new("order-1", "qt_1", 412000m, "COP", "ABC123", "1020304050", 2020, "11001");

    [Fact]
    public void Valid_request_becomes_a_command_with_the_same_data()
    {
        ValidationOutcome outcome = IssuePolicyValidator.Validate(ValidRequest, CurrentYear);

        ValidationOutcome.Valid valid = Assert.IsType<ValidationOutcome.Valid>(outcome);
        Assert.Equal(new IssuePolicyCommand("order-1", "qt_1", 412000m, "COP", "ABC123", "1020304050", 2020, "11001"), valid.Command);
    }

    public static TheoryData<string, IssuePolicyRequest> InvalidRequests => new()
    {
        { "externalRef", ValidRequest with { ExternalRef = null } },
        { "externalRef", ValidRequest with { ExternalRef = "  " } },
        { "externalRef", ValidRequest with { ExternalRef = new string('x', 65) } },
        { "quoteId", ValidRequest with { QuoteId = "" } },
        { "premium", ValidRequest with { Premium = null } },
        { "premium", ValidRequest with { Premium = 0m } },
        { "premium", ValidRequest with { Premium = -1m } },
        { "premium", ValidRequest with { Premium = 412000.555m } },
        { "currency", ValidRequest with { Currency = "cop" } },
        { "currency", ValidRequest with { Currency = "PESOS" } },
        { "currency", ValidRequest with { Currency = "COP\n" } },
        { "plate", ValidRequest with { Plate = "ABC12" } },
        { "plate", ValidRequest with { Plate = "abc123" } },
        { "plate", ValidRequest with { Plate = null } },
        { "plate", ValidRequest with { Plate = "ABC123\n" } }, // en .NET, $ aceptaría el salto de línea final
        { "documentId", ValidRequest with { DocumentId = "1234" } },
        { "documentId", ValidRequest with { DocumentId = "12345678901" } },
        { "documentId", ValidRequest with { DocumentId = "10203A4050" } },
        { "documentId", ValidRequest with { DocumentId = "12345\n" } },
        { "vehicleYear", ValidRequest with { VehicleYear = null } },
        { "vehicleYear", ValidRequest with { VehicleYear = 1979 } },
        { "vehicleYear", ValidRequest with { VehicleYear = 2028 } },
        { "cityCode", ValidRequest with { CityCode = "1100" } },
        { "cityCode", ValidRequest with { CityCode = "BOG01" } },
        { "cityCode", ValidRequest with { CityCode = "11001\n" } },
    };

    [Theory]
    [MemberData(nameof(InvalidRequests))]
    public void Each_invalid_field_is_reported_by_name(string field, IssuePolicyRequest request)
    {
        ValidationOutcome outcome = IssuePolicyValidator.Validate(request, CurrentYear);

        ValidationOutcome.Invalid invalid = Assert.IsType<ValidationOutcome.Invalid>(outcome);
        Assert.Equal([field], invalid.Errors.Keys);
    }

    [Theory]
    [InlineData(IssuePolicyValidator.MinVehicleYear)]
    [InlineData(CurrentYear + 1)]
    [InlineData(2020)]
    public void Vehicle_year_accepts_the_bounds(int year)
    {
        ValidationOutcome outcome = IssuePolicyValidator.Validate(ValidRequest with { VehicleYear = year }, CurrentYear);

        Assert.IsType<ValidationOutcome.Valid>(outcome);
    }

    [Theory]
    [InlineData("ABC12D")]
    [InlineData("XYZ987")]
    public void Plate_accepts_cars_and_motorcycles(string plate)
    {
        ValidationOutcome outcome = IssuePolicyValidator.Validate(ValidRequest with { Plate = plate }, CurrentYear);

        Assert.IsType<ValidationOutcome.Valid>(outcome);
    }

    [Fact]
    public void All_errors_are_reported_at_once()
    {
        IssuePolicyRequest empty = new(null, null, null, null, null, null, null, null);

        ValidationOutcome outcome = IssuePolicyValidator.Validate(empty, CurrentYear);

        ValidationOutcome.Invalid invalid = Assert.IsType<ValidationOutcome.Invalid>(outcome);
        Assert.Equal(
            ["cityCode", "currency", "documentId", "externalRef", "plate", "premium", "quoteId", "vehicleYear"],
            invalid.Errors.Keys.Order());
    }
}
