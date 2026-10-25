# Etapa 1: restaura y publica con el SDK.
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
WORKDIR /src
COPY global.json Directory.Build.props .editorconfig ./
COPY src/Issuing.Api/Issuing.Api.csproj src/Issuing.Api/
RUN dotnet restore src/Issuing.Api/Issuing.Api.csproj
COPY src/ src/
RUN dotnet publish src/Issuing.Api/Issuing.Api.csproj -c Release -o /app --no-restore

# Etapa 2: solo el runtime de ASP.NET y la aplicación publicada, con el usuario sin privilegios de la imagen.
FROM mcr.microsoft.com/dotnet/aspnet:10.0
WORKDIR /app
COPY --from=build /app .
USER $APP_UID
ENV ASPNETCORE_HTTP_PORTS=8080
EXPOSE 8080
ENTRYPOINT ["dotnet", "Issuing.Api.dll"]
