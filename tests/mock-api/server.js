import http from 'http';
import url from 'url';

const PORT = process.env.PORT || 3000;
const RESPONSE_DELAY_MS = parseInt(process.env.RESPONSE_DELAY_MS || '50', 10);
const SUCCESS_RATE = parseFloat(process.env.SUCCESS_RATE || '0.98');

// Helper to simulate API latency
const delay = (ms) => new Promise(resolve => setTimeout(resolve, ms));

// Helper to generate random failure
const shouldFail = () => Math.random() > SUCCESS_RATE;

// Health check endpoint
const handleHealth = () => {
  return {
    status: 200,
    body: JSON.stringify({ status: 'ok', timestamp: new Date().toISOString() })
  };
};

// Mock chat completions endpoint (Anthropic API compatible)
const handleChatCompletions = (body) => {
  if (shouldFail()) {
    return {
      status: 500,
      body: JSON.stringify({
        error: {
          type: 'api_error',
          message: 'Simulated API error'
        }
      })
    };
  }

  const req = JSON.parse(body);
  const messages = req.messages || [];
  const content = messages.length > 0 ? messages[0].content : 'Hello, world!';

  return {
    status: 200,
    body: JSON.stringify({
      id: `msg_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      type: 'message',
      role: 'assistant',
      content: [
        {
          type: 'text',
          text: `Mock response to: "${content.substring(0, 50)}..."`,
        }
      ],
      model: req.model || 'claude-3-sonnet-20240229',
      stop_reason: 'end_turn',
      stop_sequence: null,
      usage: {
        input_tokens: content.length > 4 ? Math.ceil(content.length / 4) : 10,
        output_tokens: 50
      }
    })
  };
};

// Mock models list endpoint
const handleListModels = () => {
  return {
    status: 200,
    body: JSON.stringify({
      object: 'list',
      data: [
        {
          id: 'claude-3-haiku-20240307',
          object: 'model',
          created: 1707973000,
          owned_by: 'anthropic'
        },
        {
          id: 'claude-3-sonnet-20240229',
          object: 'model',
          created: 1707973000,
          owned_by: 'anthropic'
        },
        {
          id: 'claude-3-opus-20240229',
          object: 'model',
          created: 1707973000,
          owned_by: 'anthropic'
        }
      ]
    })
  };
};

// Mock batch submit endpoint
const handleBatchSubmit = (body) => {
  const req = JSON.parse(body);
  return {
    status: 200,
    body: JSON.stringify({
      id: `batch_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      type: 'batch',
      processing_status: 'queued',
      request_counts: {
        processing: req.requests?.length || 0,
        succeeded: 0,
        errored: 0,
        canceled: 0,
        expired: 0
      },
      created_at: new Date().toISOString(),
      expires_at: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString()
    })
  };
};

// Mock batch retrieve endpoint
const handleBatchRetrieve = (batchId) => {
  return {
    status: 200,
    body: JSON.stringify({
      id: batchId,
      type: 'batch',
      processing_status: 'succeeded',
      request_counts: {
        processing: 0,
        succeeded: 10,
        errored: 0,
        canceled: 0,
        expired: 0
      },
      created_at: new Date(Date.now() - 60000).toISOString(),
      expires_at: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
      results_url: `/v1/batch/${batchId}/results`
    })
  };
};

// Main request handler
const requestHandler = async (req, res) => {
  const parsedUrl = url.parse(req.url, true);
  const pathname = parsedUrl.pathname;

  // Add CORS headers
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  res.setHeader('Content-Type', 'application/json');

  // Handle preflight requests
  if (req.method === 'OPTIONS') {
    res.writeHead(200);
    res.end();
    return;
  }

  // Simulate API latency
  await delay(RESPONSE_DELAY_MS);

  let response;

  try {
    if (pathname === '/health') {
      response = handleHealth();
    } else if (pathname === '/v1/messages' && req.method === 'POST') {
      const body = await readBody(req);
      response = handleChatCompletions(body);
    } else if (pathname === '/v1/models' && req.method === 'GET') {
      response = handleListModels();
    } else if (pathname === '/v1/batches' && req.method === 'POST') {
      const body = await readBody(req);
      response = handleBatchSubmit(body);
    } else if (pathname.match(/^\/v1\/batches\/[a-z0-9_-]+$/i) && req.method === 'GET') {
      const batchId = pathname.split('/')[3];
      response = handleBatchRetrieve(batchId);
    } else {
      response = {
        status: 404,
        body: JSON.stringify({
          error: {
            type: 'not_found_error',
            message: `Endpoint not found: ${pathname}`
          }
        })
      };
    }
  } catch (err) {
    console.error('Request handler error:', err);
    response = {
      status: 500,
      body: JSON.stringify({
        error: {
          type: 'internal_server_error',
          message: err.message
        }
      })
    };
  }

  res.writeHead(response.status);
  res.end(response.body);
};

// Helper to read request body
const readBody = (req) => {
  return new Promise((resolve, reject) => {
    let body = '';
    req.on('data', chunk => {
      body += chunk.toString();
    });
    req.on('end', () => {
      try {
        resolve(body);
      } catch (e) {
        reject(e);
      }
    });
    req.on('error', reject);
  });
};

// Create and start server
const server = http.createServer(requestHandler);

server.listen(PORT, () => {
  console.log(`Mock Anthropic API server running on port ${PORT}`);
  console.log(`Response delay: ${RESPONSE_DELAY_MS}ms`);
  console.log(`Success rate: ${(SUCCESS_RATE * 100).toFixed(1)}%`);
  console.log(`Health check: http://localhost:${PORT}/health`);
});

// Graceful shutdown
process.on('SIGTERM', () => {
  console.log('SIGTERM received, shutting down gracefully');
  server.close(() => {
    console.log('Server closed');
    process.exit(0);
  });
});
