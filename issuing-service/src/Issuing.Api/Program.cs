using Issuing.Api.Application;
using Issuing.Api.Http;
using Issuing.Api.Legacy;
using Microsoft.Extensions.Options;

WebApplicationBuilder builder = WebApplication.CreateBuilder(args);

// Logs JSON con el scope (CorrelationId) en cada línea.
builder.Logging.ClearProviders();
builder.Logging.AddJsonConsole(options => options.IncludeScopes = true);

builder.Services.AddOptions<LegacyIssuerOptions>()
    .Bind(builder.Configuration.GetSection(LegacyIssuerOptions.Section))
    .ValidateDataAnnotations()
    .Validate(o => o.WorstCase < LegacyIssuerOptions.CallerBudget, "IssueTimeout + 2 × LookupTimeout must stay under the orchestrator's 10 s per attempt")
    .ValidateOnStart();
builder.Services.AddHttpClient<LegacyIssuerClient>((services, http) =>
{
    Uri baseUrl = services.GetRequiredService<IOptions<LegacyIssuerOptions>>().Value.BaseUrl
        ?? throw new InvalidOperationException("LegacyIssuer:BaseUrl is required");
    // Con la barra final, "issue" y "policies" se resuelven debajo de la ruta base y no la reemplazan.
    http.BaseAddress = baseUrl.AbsoluteUri.EndsWith('/') ? baseUrl : new Uri(baseUrl.AbsoluteUri + "/");
});
builder.Services.AddSingleton(TimeProvider.System);
builder.Services.AddScoped<IssuePolicyHandler>();

WebApplication app = builder.Build();

app.UseCorrelationId();
app.MapGet("/health", () => Results.Ok(new HealthResponse("ok")));
app.MapPolicyEndpoints();

app.Run();

/// <summary>Estado del servicio para sondas de vida.</summary>
internal sealed record HealthResponse(string Status);

/// <summary>Punto de entrada; parcial para exponerlo a WebApplicationFactory en las pruebas.</summary>
public partial class Program;
