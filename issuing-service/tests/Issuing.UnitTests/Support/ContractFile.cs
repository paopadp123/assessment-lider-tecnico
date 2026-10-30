using System.Text.Json;

namespace Issuing.UnitTests.Support;

/// <summary>Un contrato de contracts/ (raíz del repositorio). El formato está en contracts/README.md.</summary>
public sealed class ContractFile
{
    private ContractFile(IReadOnlyList<ContractInteraction> interactions) => Interactions = interactions;

    public IReadOnlyList<ContractInteraction> Interactions { get; }

    /// <summary>Lee contracts/{name} subiendo desde la carpeta de la prueba hasta encontrarla.</summary>
    public static ContractFile Load(string name)
    {
        DirectoryInfo? dir = new(AppContext.BaseDirectory);
        while (dir is not null)
        {
            string path = Path.Combine(dir.FullName, "contracts", name);
            if (File.Exists(path))
            {
                using JsonDocument doc = JsonDocument.Parse(File.ReadAllText(path));
                List<ContractInteraction> interactions = [.. doc.RootElement.GetProperty("interactions").EnumerateArray().Select(ContractInteraction.From)];
                return interactions.Count == 0
                    ? throw new InvalidOperationException($"contrato {name} sin interacciones")
                    : new ContractFile(interactions);
            }

            dir = dir.Parent;
        }

        throw new FileNotFoundException($"no se encontró contracts/{name} subiendo desde {AppContext.BaseDirectory}");
    }

    public ContractInteraction Named(string name) => Interactions.Single(i => i.Name == name);

    /// <summary>
    /// Compara actual contra el ejemplo del contrato: los mismos campos con el mismo tipo JSON (actual puede tener
    /// campos de más) y el mismo valor en las rutas de exact. Un arreglo vacío en el ejemplo exige un arreglo vacío.
    /// </summary>
    public static IReadOnlyList<string> Match(JsonElement expected, string actualJson, IReadOnlyList<string> exact)
    {
        JsonDocument actual;
        try
        {
            actual = JsonDocument.Parse(actualJson);
        }
        catch (JsonException ex)
        {
            return [$"cuerpo que no es JSON: {ex.Message}"];
        }

        using (actual)
        {
            List<string> problems = [];
            Shape("$", expected, actual.RootElement, problems);
            foreach (string path in exact)
            {
                JsonElement? want = Lookup(expected, path);
                JsonElement? got = Lookup(actual.RootElement, path);
                if (want is null)
                {
                    problems.Add($"{path}: está en exact pero no en el ejemplo");
                }
                else if (got is null || !SameValue(want.Value, got.Value))
                {
                    problems.Add($"{path}: se esperaba {want.Value.GetRawText()}, llegó {got?.GetRawText() ?? "nada"}");
                }
            }

            return problems;
        }
    }

    /// <summary>
    /// Compara la solicitud del consumidor con la del contrato por forma y, salvo los campos dinámicos, por valor:
    /// el proveedor recibe exactamente esos valores (cambiar la placa por el documento, ambos texto, también rompe).
    /// </summary>
    public static IReadOnlyList<string> MatchRequest(ContractInteraction it, string actualJson)
    {
        JsonElement expected = it.RequestBody ?? throw new InvalidOperationException($"{it.Name} no tiene cuerpo de solicitud");
        IReadOnlyList<string> problems = Match(expected, actualJson, []);
        if (problems.Count > 0)
        {
            return problems;
        }

        using JsonDocument actual = JsonDocument.Parse(actualJson);
        List<string> differences = [];
        Values(string.Empty, expected, actual.RootElement, it.Dynamic, differences);
        return differences;
    }

    private static void Values(string path, JsonElement want, JsonElement got, IReadOnlyList<string> dynamic, List<string> problems)
    {
        if (dynamic.Contains(path))
        {
            return;
        }

        switch (want.ValueKind)
        {
            case JsonValueKind.Object:
                foreach (JsonProperty property in want.EnumerateObject())
                {
                    string child = path.Length == 0 ? property.Name : $"{path}.{property.Name}";
                    Values(child, property.Value, got.GetProperty(property.Name), dynamic, problems);
                }

                break;
            case JsonValueKind.Array:
                for (int i = 0; i < Math.Min(want.GetArrayLength(), got.GetArrayLength()); i++)
                {
                    Values($"{path}[{i}]", want[i], got[i], dynamic, problems);
                }

                break;
            default:
                if (!SameValue(want, got))
                {
                    problems.Add($"{path}: se esperaba {want.GetRawText()}, llegó {got.GetRawText()}");
                }

                break;
        }
    }

    // 412000.0 y 412000 son el mismo número JSON, como en los comparadores de Go y TypeScript.
    private static bool SameValue(JsonElement want, JsonElement got) =>
        want.ValueKind == JsonValueKind.Number && got.ValueKind == JsonValueKind.Number
            ? want.GetDecimal() == got.GetDecimal()
            : want.GetRawText() == got.GetRawText();

    private static void Shape(string path, JsonElement want, JsonElement got, List<string> problems)
    {
        if (Kind(want) != Kind(got))
        {
            problems.Add($"{path}: se esperaba {Kind(want)}, llegó {Kind(got)}");
            return;
        }

        if (want.ValueKind == JsonValueKind.Object)
        {
            foreach (JsonProperty property in want.EnumerateObject())
            {
                if (got.TryGetProperty(property.Name, out JsonElement child))
                {
                    Shape($"{path}.{property.Name}", property.Value, child, problems);
                }
                else
                {
                    problems.Add($"{path}.{property.Name}: falta");
                }
            }
        }
        else if (want.ValueKind == JsonValueKind.Array)
        {
            if (want.GetArrayLength() == 0)
            {
                if (got.GetArrayLength() != 0)
                {
                    problems.Add($"{path}: se esperaba un arreglo vacío, llegaron {got.GetArrayLength()} elementos");
                }

                return;
            }

            int index = 0;
            foreach (JsonElement item in got.EnumerateArray())
            {
                Shape($"{path}[{index++}]", want[0], item, problems);
            }
        }
    }

    // true y false son el mismo tipo JSON (booleano).
    private static JsonValueKind Kind(JsonElement element) =>
        element.ValueKind == JsonValueKind.False ? JsonValueKind.True : element.ValueKind;

    private static JsonElement? Lookup(JsonElement element, string path)
    {
        foreach (string key in path.Split('.'))
        {
            if (element.ValueKind != JsonValueKind.Object || !element.TryGetProperty(key, out element))
            {
                return null;
            }
        }

        return element;
    }
}

/// <summary>Una solicitud del consumidor y la respuesta que necesita.</summary>
public sealed record ContractInteraction(
    string Name,
    string ProviderState,
    string Method,
    string Path,
    IReadOnlyDictionary<string, string> Query,
    JsonElement? RequestBody,
    IReadOnlyList<string> Dynamic,
    int Status,
    JsonElement? ResponseBody,
    IReadOnlyList<string> Exact,
    string ConsumerExpects)
{
    /// <summary>Ruta con su query, tal como la envía el consumidor.</summary>
    public string Url => Query.Count == 0
        ? Path
        : $"{Path}?{string.Join('&', Query.Select(q => $"{q.Key}={Uri.EscapeDataString(q.Value)}"))}";

    public override string ToString() => Name;

    internal static ContractInteraction From(JsonElement item)
    {
        JsonElement request = item.GetProperty("request");
        JsonElement response = item.GetProperty("response");
        Dictionary<string, string> query = request.TryGetProperty("query", out JsonElement q)
            ? q.EnumerateObject().ToDictionary(p => p.Name, p => p.Value.GetString() ?? string.Empty)
            : [];
        List<string> exact = item.TryGetProperty("exact", out JsonElement e) ? [.. e.EnumerateArray().Select(x => x.GetString() ?? string.Empty)] : [];
        List<string> dynamic = request.TryGetProperty("dynamic", out JsonElement d) ? [.. d.EnumerateArray().Select(x => x.GetString() ?? string.Empty)] : [];
        return new ContractInteraction(
            item.GetProperty("name").GetString() ?? string.Empty,
            item.GetProperty("providerState").GetString() ?? string.Empty,
            request.GetProperty("method").GetString() ?? string.Empty,
            request.GetProperty("path").GetString() ?? string.Empty,
            query,
            request.TryGetProperty("body", out JsonElement reqBody) ? reqBody.Clone() : null,
            dynamic,
            response.GetProperty("status").GetInt32(),
            response.TryGetProperty("body", out JsonElement resBody) ? resBody.Clone() : null,
            exact,
            item.GetProperty("consumerExpects").GetString() ?? string.Empty);
    }
}
