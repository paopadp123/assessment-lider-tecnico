using System.Net;
using Issuing.UnitTests.Support;

namespace Issuing.UnitTests;

/// <summary>Verifica que el servicio arranca y responde la sonda de vida.</summary>
public sealed class HealthEndpointTests
{
    [Fact]
    public async Task Health_ReturnsOk()
    {
        using IssuingApp app = new();
        using HttpClient client = app.CreateClient();

        using HttpResponseMessage response = await client.GetAsync(new Uri("/health", UriKind.Relative));

        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Equal("{\"status\":\"ok\"}", await response.Content.ReadAsStringAsync());
    }
}
