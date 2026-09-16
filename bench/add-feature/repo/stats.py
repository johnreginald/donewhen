def mean(values):
    if not values:
        raise ValueError("empty")
    return sum(values) / len(values)
