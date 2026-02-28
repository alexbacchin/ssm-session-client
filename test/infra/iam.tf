# ---------------------------------------------------------------------------
# IAM role for test EC2 instances
# ---------------------------------------------------------------------------

resource "aws_iam_role" "test_instance" {
  name        = "${var.name_prefix}-instance-role"
  description = "Role assumed by ssm-session-client acceptance test EC2 instances."

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "ec2.amazonaws.com" }
    }]
  })

  tags = {
    Name        = "${var.name_prefix}-instance-role"
    Environment = var.environment
  }
}

resource "aws_iam_role_policy_attachment" "ssm_managed_core" {
  role       = aws_iam_role.test_instance.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# Allow the instance to receive EC2 Instance Connect public keys.
resource "aws_iam_role_policy" "instance_connect" {
  name = "${var.name_prefix}-instance-connect"
  role = aws_iam_role.test_instance.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "ec2-instance-connect:SendSSHPublicKey"
      Resource = "arn:aws:ec2:${var.region}:${data.aws_caller_identity.current.account_id}:instance/*"
      Condition = {
        StringEquals = { "ec2:osuser" = "ec2-user" }
      }
    }]
  })
}

resource "aws_iam_instance_profile" "test_instance" {
  name = "${var.name_prefix}-instance-profile"
  role = aws_iam_role.test_instance.name
}

# ---------------------------------------------------------------------------
# IAM policy for the test runner (local user or CI role)
# ---------------------------------------------------------------------------

resource "aws_iam_policy" "test_runner" {
  name        = "${var.name_prefix}-test-runner"
  description = "Minimum permissions for ssm-session-client acceptance test runner."

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "SSMSession"
        Effect = "Allow"
        Action = [
          "ssm:StartSession",
          "ssm:TerminateSession",
          "ssm:DescribeSessions",
          "ssm:GetConnectionStatus",
          "ssm:DescribeInstanceInformation",
          "ssm:ListTagsForResource",
        ]
        Resource = "*"
      },
      {
        Sid      = "EC2Describe"
        Effect   = "Allow"
        Action   = ["ec2:DescribeInstances"]
        Resource = "*"
      },
      {
        Sid    = "EC2InstanceConnect"
        Effect = "Allow"
        Action = ["ec2-instance-connect:SendSSHPublicKey"]
        Resource = "arn:aws:ec2:${var.region}:${data.aws_caller_identity.current.account_id}:instance/*"
      },
      {
        Sid      = "KMS"
        Effect   = "Allow"
        Action   = ["kms:GenerateDataKey", "kms:Decrypt"]
        Resource = "*"
      },
      {
        Sid      = "SSMMessages"
        Effect   = "Allow"
        Action   = ["ssmmessages:CreateControlChannel", "ssmmessages:CreateDataChannel",
          "ssmmessages:OpenControlChannel", "ssmmessages:OpenDataChannel"]
        Resource = "*"
      }
    ]
  })
}

# ---------------------------------------------------------------------------
# Optional: GitHub Actions OIDC role
# ---------------------------------------------------------------------------

data "aws_iam_openid_connect_provider" "github" {
  count = var.github_org != "" ? 1 : 0
  url   = "https://token.actions.githubusercontent.com"
}

resource "aws_iam_role" "github_actions" {
  count       = var.github_org != "" ? 1 : 0
  name        = "${var.name_prefix}-github-actions"
  description = "Role assumed by GitHub Actions via OIDC for acceptance tests."

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = data.aws_iam_openid_connect_provider.github[0].arn }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringLike = {
          "token.actions.githubusercontent.com:sub" = "repo:${var.github_org}/${var.github_repo}:*"
        }
        StringEquals = {
          "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
        }
      }
    }]
  })

  tags = {
    Name        = "${var.name_prefix}-github-actions"
    Environment = var.environment
  }
}

resource "aws_iam_role_policy_attachment" "github_actions" {
  count      = var.github_org != "" ? 1 : 0
  role       = aws_iam_role.github_actions[0].name
  policy_arn = aws_iam_policy.test_runner.arn
}

output "github_actions_role_arn" {
  description = "ARN of the GitHub Actions OIDC role (empty if github_org not set)."
  value       = var.github_org != "" ? aws_iam_role.github_actions[0].arn : ""
}
