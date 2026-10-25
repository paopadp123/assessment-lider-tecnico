using System.Text.RegularExpressions;

namespace Issuing.Api.Http;

/// <summary>
/// Toma el X-Correlation-ID que envía el orquestador (o genera uno si falta o es inválido), lo devuelve en la
/// respuesta y lo pone en el scope de logging: todos los logs de la solicitud lo llevan.
/// </summary>
public static partial class Correlation
{
    public const string Header = "X-Correlation-ID";

    private const string ItemKey = "CorrelationId";

    private static readonly Func<ILogger, string, IDisposable?> CorrelationScope =
        LoggerMessage.DefineScope<string>("CorrelationId:{CorrelationId}");

    public static IApplicationBuilder UseCorrelationId(this IApplicationBuilder app) =>
        app.Use(async (context, next) =>
        {
            string received = context.Request.Headers[Header].ToString();
            string id = IdFormat().IsMatch(received) ? received : Guid.NewGuid().ToString();
            context.Items[ItemKey] = id;
            context.Response.Headers[Header] = id;

            ILogger logger = context.RequestServices.GetRequiredService<ILoggerFactory>().CreateLogger(typeof(Correlation));
            using (CorrelationScope(logger, id))
            {
                await next(context);
            }
        });

    public static string Id(HttpContext context)
    {
        ArgumentNullException.ThrowIfNull(context);
        return context.Items[ItemKey] as string ?? Guid.NewGuid().ToString();
    }

    // Mismo formato que acepta el orquestador: un valor con saltos de línea no llega a los logs.
    [GeneratedRegex(@"^[A-Za-z0-9._:-]{1,64}\z")]
    private static partial Regex IdFormat();
}
