const express = require("express");
const app = express();
const PORT = process.env.PORT || 9090;

app.use(express.json());

// Generic endpoint that receives any payload and returns a mock response.
// Logs the full request body for testing/debugging (including revealed PAN/CVV).
app.post("/receive", (req, res) => {
  console.log(
    JSON.stringify({
      timestamp: new Date().toISOString(),
      method: req.method,
      path: req.path,
      headers: req.headers,
      body: req.body,
    })
  );

  res.json({
    status: "success",
    provider_tx_id: `mock_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`,
    message: "Mock provider received payload",
  });
});

// Health check
app.get("/health", (_req, res) => {
  res.json({ status: "healthy" });
});

app.listen(PORT, () => {
  console.log(`Mock provider listening on port ${PORT}`);
});
