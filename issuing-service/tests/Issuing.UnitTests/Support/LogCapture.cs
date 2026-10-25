using System.Collections.Concurrent;
using System.Text;
using Microsoft.Extensions.Logging;

namespace Issuing.UnitTests.Support;

/// <summary>Guarda cada línea de log con sus scopes, para verificar qué queda escrito.</summary>
public sealed class LogCapture : ILoggerProvider, ISupportExternalScope
{
    private readonly ConcurrentQueue<string> _lines = new();
    private IExternalScopeProvider _scopes = new LoggerExternalScopeProvider();

    public string All => string.Join('\n', _lines);

    public ILogger CreateLogger(string categoryName) => new Logger(this);

    public void SetScopeProvider(IExternalScopeProvider scopeProvider) => _scopes = scopeProvider;

    public void Dispose()
    {
    }

    private sealed class Logger(LogCapture owner) : ILogger
    {
        public IDisposable? BeginScope<TState>(TState state)
            where TState : notnull => owner._scopes.Push(state);

        public bool IsEnabled(LogLevel logLevel) => true;

        public void Log<TState>(LogLevel logLevel, EventId eventId, TState state, Exception? exception, Func<TState, Exception?, string> formatter)
        {
            StringBuilder line = new(formatter(state, exception));
            owner._scopes.ForEachScope(
                (scope, builder) =>
                {
                    if (scope is IEnumerable<KeyValuePair<string, object>> pairs)
                    {
                        foreach (KeyValuePair<string, object> pair in pairs)
                        {
                            builder.Append($" {pair.Key}={pair.Value}");
                        }
                    }
                },
                line);
            owner._lines.Enqueue(line.ToString());
        }
    }
}
