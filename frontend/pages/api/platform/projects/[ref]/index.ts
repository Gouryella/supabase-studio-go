import { NextApiRequest, NextApiResponse } from 'next'

import apiWrapper from 'lib/api/apiWrapper'
import { DEFAULT_PROJECT, PROJECT_REST_URL } from 'lib/constants/api'

export default (req: NextApiRequest, res: NextApiResponse) => apiWrapper(req, res, handler)

let projectName = DEFAULT_PROJECT.name

async function handler(req: NextApiRequest, res: NextApiResponse) {
  const { method } = req

  switch (method) {
    case 'GET':
      return handleGet(req, res)
    case 'PATCH':
      return handlePatch(req, res)
    default:
      res.setHeader('Allow', ['GET', 'PATCH'])
      res.status(405).json({ data: null, error: { message: `Method ${method} Not Allowed` } })
  }
}

const handleGet = async (req: NextApiRequest, res: NextApiResponse) => {
  // Platform specific endpoint
  const response = {
    ...DEFAULT_PROJECT,
    name: projectName,
    connectionString: '',
    restUrl: PROJECT_REST_URL,
  }

  return res.status(200).json(response)
}

const handlePatch = async (req: NextApiRequest, res: NextApiResponse) => {
  let payload: any = req.body
  if (typeof req.body === 'string') {
    try {
      payload = JSON.parse(req.body)
    } catch {
      return res.status(400).json({ data: null, error: { message: 'Invalid request body' } })
    }
  }

  const name = typeof payload?.name === 'string' ? payload.name.trim() : ''

  if (name.length < 3) {
    return res.status(400).json({
      data: null,
      error: { message: 'Project name must be at least 3 characters long' },
    })
  }

  if (name.length > 64) {
    return res.status(400).json({
      data: null,
      error: { message: 'Project name must be at most 64 characters long' },
    })
  }

  projectName = name

  return res.status(200).json({
    id: DEFAULT_PROJECT.id,
    ref: DEFAULT_PROJECT.ref,
    name: projectName,
  })
}
