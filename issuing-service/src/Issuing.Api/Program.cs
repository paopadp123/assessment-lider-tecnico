WebApplicationBuilder builder = WebApplication.CreateBuilder(args);
WebApplication app = builder.Build();

app.MapGet("/health", () => Results.Ok(new HealthResponse("ok")));

app.Run();

/// <summary>Estado del servicio para sondas de vida.</summary>
internal sealed record HealthResponse(string Status);

/// <summary>Punto de entrada; parcial para exponerlo a WebApplicationFactory en las pruebas.</summary>
public partial class Program;
