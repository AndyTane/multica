export function BlockingRuntimeConfigError({ message }: { message: string }) {
  return (
    <div className="flex h-screen items-center justify-center bg-background p-8 text-foreground">
      <div className="max-w-xl rounded-lg border bg-card p-6 shadow-sm">
        <h1 className="text-title font-semibold">Desktop configuration error</h1>
        <p className="mt-3 text-body text-muted-foreground">
          Create or correct the configuration file shown below, then restart the app.
        </p>
        <pre className="mt-4 whitespace-pre-wrap break-all rounded-md bg-muted p-3 text-caption text-muted-foreground">
          {message}
        </pre>
      </div>
    </div>
  );
}
