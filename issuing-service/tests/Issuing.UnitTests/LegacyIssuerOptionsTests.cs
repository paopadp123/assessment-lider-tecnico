using Issuing.Api.Legacy;
using Issuing.UnitTests.Support;
using Microsoft.AspNetCore.Hosting;
using Microsoft.Extensions.Options;

namespace Issuing.UnitTests;

public sealed class LegacyIssuerOptionsTests
{
    [Fact]
    public void Default_timeouts_fit_in_the_orchestrator_attempt()
    {
        LegacyIssuerOptions defaults = new();

        Assert.True(defaults.WorstCase < LegacyIssuerOptions.CallerBudget, $"peor caso {defaults.WorstCase}");
    }

    [Fact]
    public void Timeouts_that_exceed_the_orchestrator_attempt_fail_at_startup()
    {
        using IssuingApp app = new();
        using Microsoft.AspNetCore.Mvc.Testing.WebApplicationFactory<Program> slow =
            app.WithWebHostBuilder(b => b.UseSetting("LegacyIssuer:IssueTimeout", "00:00:09.500"));

        Assert.Throws<OptionsValidationException>(() => slow.CreateClient());
    }
}
