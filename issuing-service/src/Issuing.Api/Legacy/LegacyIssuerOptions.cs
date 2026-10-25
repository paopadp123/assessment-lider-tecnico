using System.ComponentModel.DataAnnotations;

namespace Issuing.Api.Legacy;

/// <summary>
/// Configuración del emisor legado (sección "LegacyIssuer"; por variables de entorno: LegacyIssuer__BaseUrl…).
/// La consulta previa + la emisión + la consulta posterior (2 + 5 + 2 s por defecto) deben caber en los 10 s
/// que el orquestador espera por intento.
/// </summary>
public sealed class LegacyIssuerOptions
{
    public const string Section = "LegacyIssuer";

    [Required]
    public Uri? BaseUrl { get; set; }

    [Range(typeof(TimeSpan), "00:00:00.001", "00:00:30")]
    public TimeSpan IssueTimeout { get; set; } = TimeSpan.FromSeconds(5);

    [Range(typeof(TimeSpan), "00:00:00.001", "00:00:30")]
    public TimeSpan LookupTimeout { get; set; } = TimeSpan.FromSeconds(2);

    /// <summary>Lo que el orquestador espera por intento: el peor caso de la ACL debe quedar por debajo.</summary>
    public static readonly TimeSpan CallerBudget = TimeSpan.FromSeconds(10);

    public TimeSpan WorstCase => IssueTimeout + LookupTimeout + LookupTimeout;
}
