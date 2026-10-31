-- CLAWSTA-85 batch A: the workspace's built-in Chief of Staff displays as
-- Siman instead of Mika.
--
-- The row is identified by system_key ('mika'), never by name: that key is the
-- server-side identity and stays put, and an owner is free to rename the agent.
-- Only the name the product shipped is migrated, so an owner who already picked
-- another name keeps it. The seeded description is product copy that also
-- named the old display name, so it is migrated only when it still equals one of
-- the strings the server wrote at creation; an owner-edited description is left
-- alone. Idempotent: after the first run the WHERE matches nothing.
UPDATE agent
SET name = CASE WHEN name = 'Mika' THEN 'Siman' ELSE name END,
    description = CASE
        WHEN description IN (
            'Your workspace Chief of Staff. Mika turns goals into issues, coordinates agents, and helps build reusable workflows.',
            '你的工作区 Chief of Staff。Mika 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。',
            '워크스페이스의 Chief of Staff입니다. Mika가 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다.',
            'ワークスペースの Chief of Staff。Mika は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。'
        ) THEN REPLACE(description, 'Mika', 'Siman')
        ELSE description
    END,
    updated_at = now()
WHERE system_key = 'mika'
  AND (
      name = 'Mika'
      OR description IN (
          'Your workspace Chief of Staff. Mika turns goals into issues, coordinates agents, and helps build reusable workflows.',
          '你的工作区 Chief of Staff。Mika 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。',
          '워크스페이스의 Chief of Staff입니다. Mika가 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다.',
          'ワークスペースの Chief of Staff。Mika は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。'
      )
  );
