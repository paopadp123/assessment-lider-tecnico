using Issuing.Api.Legacy;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.AspNetCore.TestHost;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;

namespace Issuing.UnitTests.Support;

/// <summary>La ACL completa en memoria, con el emisor legado falso y los logs capturados.</summary>
public sealed class IssuingApp : WebApplicationFactory<Program>
{
    public FakeLegacyIssuer Legacy { get; } = new();

    public LogCapture Logs { get; } = new();

    protected override void ConfigureWebHost(IWebHostBuilder builder)
    {
        builder.UseSetting("LegacyIssuer:BaseUrl", "http://legacy.test/");
        // Tiempos cortos: el emisor que "no responde" se corta rápido en las pruebas.
        builder.UseSetting("LegacyIssuer:IssueTimeout", "00:00:00.300");
        builder.UseSetting("LegacyIssuer:LookupTimeout", "00:00:00.300");
        builder.ConfigureTestServices(services =>
        {
            services.AddHttpClient<LegacyIssuerClient>().ConfigurePrimaryHttpMessageHandler(() => Legacy);
            services.AddSingleton<ILoggerProvider>(Logs);
        });
    }
}
